package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	modelsRepo "github.com/ArtemVladimirov/broadlinkac2mqtt/app/repository/models"
	"github.com/ArtemVladimirov/broadlinkac2mqtt/app/service/models"
)

const (
	minUdpGap        = 500 * time.Millisecond
	commandDebounce  = 300 * time.Millisecond
	settleAfterSet   = 500 * time.Millisecond
	ambientInterval  = 3 * time.Minute
	offlineFailCount = 3
	defaultPoll      = 10 * time.Second
	minBackoff       = 2 * time.Second
	minBackoffCap    = 30 * time.Second
)

type deviceMonitor struct {
	s            *service
	mac          string
	pollInterval time.Duration
	offlineAfter time.Duration
	backoffCap   time.Duration

	modeUpdatedTime        time.Time
	swingModeUpdatedTime   time.Time
	fanModeUpdatedTime     time.Time
	temperatureUpdatedTime time.Time
	isDisplayOnUpdatedTime time.Time
	isMildewOnUpdatedTime  time.Time
	isCleanOnUpdatedTime   time.Time
	isHealthOnUpdatedTime  time.Time

	lastSuccess      time.Time
	lastUDP          time.Time
	startedAt        time.Time
	consecutiveFails int
	backoffStep      int
	isOnline         bool
	hasSuccess       bool
	ambientStarted   bool
}

type pendingCommands struct {
	mode        *string
	swingMode   *string
	fanMode     *string
	temperature *float32
	isDisplayOn *bool
	isMildewOn  *bool
	isCleanOn   *bool
	isHealthOn  *bool

	modeAt     time.Time
	swingAt    time.Time
	fanAt      time.Time
	tempAt     time.Time
	displayAt  time.Time
	mildewAt   time.Time
	cleanAt    time.Time
	healthAt   time.Time
	anyPending bool
}

func (s *service) StartDeviceMonitoring(ctx context.Context, input *models.StartDeviceMonitoringInput) error {
	commandCh := make(chan struct{}, 1)
	s.commandNotify.Store(input.Mac, commandCh)
	defer s.commandNotify.Delete(input.Mac)

	pollInterval := time.Duration(s.updateInterval) * time.Second
	if pollInterval <= 0 {
		pollInterval = defaultPoll
	}

	backoffCap := minBackoffCap
	if pollInterval*3 > backoffCap {
		backoffCap = pollInterval * 3
	}

	m := &deviceMonitor{
		s:            s,
		mac:          input.Mac,
		pollInterval: pollInterval,
		offlineAfter: pollInterval * 3,
		backoffCap:   backoffCap,
		startedAt:    time.Now(),
	}

	pollTicker := time.NewTicker(pollInterval)
	defer pollTicker.Stop()

	ambientTicker := time.NewTicker(ambientInterval)
	defer ambientTicker.Stop()
	ambientTicker.Stop()

	if err := m.pollState(ctx); err != nil {
		if ctx.Err() != nil {
			return nil
		}
	}
	m.startAmbientIfReady(ctx, ambientTicker)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-commandCh:
			if err := m.applyPendingCommands(ctx, pollTicker); err != nil && ctx.Err() != nil {
				return nil
			}
			m.startAmbientIfReady(ctx, ambientTicker)
		case <-pollTicker.C:
			if err := m.pollState(ctx); err != nil && ctx.Err() != nil {
				return nil
			}
			m.startAmbientIfReady(ctx, ambientTicker)
		case <-ambientTicker.C:
			if !m.hasSuccess {
				continue
			}
			if err := m.pollAmbient(ctx); err != nil && ctx.Err() != nil {
				return nil
			}
		}
	}
}

func (m *deviceMonitor) startAmbientIfReady(ctx context.Context, ambientTicker *time.Ticker) {
	if m.ambientStarted || !m.hasSuccess || ctx.Err() != nil {
		return
	}
	if err := m.pollAmbient(ctx); err != nil && ctx.Err() != nil {
		return
	}
	ambientTicker.Reset(ambientInterval)
	m.ambientStarted = true
}

func (m *deviceMonitor) pollState(ctx context.Context) error {
	if err := m.waitMinGap(ctx); err != nil {
		return err
	}

	err := m.s.GetDeviceStates(ctx, &models.GetDeviceStatesInput{Mac: m.mac})
	m.lastUDP = time.Now()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		slog.ErrorContext(ctx, "failed to get AC States",
			slog.Any("err", err),
			slog.String("device", m.mac))
		m.onGetFailure(ctx)
		return m.sleepBackoff(ctx)
	}

	m.onGetSuccess(ctx)
	m.wakeIfPending(ctx)
	return nil
}

func (m *deviceMonitor) pollAmbient(ctx context.Context) error {
	if err := m.waitMinGap(ctx); err != nil {
		return err
	}

	err := m.s.getDeviceAmbientTemperature(ctx, &models.GetDeviceAmbientTemperatureInput{Mac: m.mac})
	m.lastUDP = time.Now()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		slog.ErrorContext(ctx, "failed to get ambient temperature",
			slog.Any("err", err),
			slog.String("device", m.mac))
	}
	return nil
}

func (m *deviceMonitor) applyPendingCommands(ctx context.Context, pollTicker *time.Ticker) error {
	if err := sleepCtx(ctx, commandDebounce); err != nil {
		return err
	}

	pending, err := m.readPending(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to read mqtt commands from cache",
			slog.Any("err", err),
			slog.String("device", m.mac))
		if err = m.sleepBackoff(ctx); err != nil {
			return err
		}
		m.s.wakeMonitor(m.mac)
		return nil
	}
	if !pending.anyPending {
		return nil
	}
	if !m.hasRawStatus(ctx) {
		return nil
	}

	if err = m.waitMinGap(ctx); err != nil {
		return err
	}

	updateDeviceStatesInput := &models.UpdateDeviceStatesInput{
		Mac:         m.mac,
		FanMode:     pending.fanMode,
		SwingMode:   pending.swingMode,
		Mode:        pending.mode,
		Temperature: pending.temperature,
		IsDisplayOn: pending.isDisplayOn,
		IsMildewOn:  pending.isMildewOn,
		IsCleanOn:   pending.isCleanOn,
		IsHealthOn:  pending.isHealthOn,
	}
	err = m.s.UpdateDeviceStates(ctx, updateDeviceStatesInput)
	m.lastUDP = time.Now()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		slog.ErrorContext(ctx, "failed to update device states",
			slog.Any("err", err),
			slog.String("device", m.mac),
			slog.Any("input", updateDeviceStatesInput))
		if err = m.sleepBackoff(ctx); err != nil {
			return err
		}
		m.s.wakeMonitor(m.mac)
		return nil
	}

	m.rememberApplied(pending)

	if err = m.waitBeforeUDP(ctx, settleAfterSet); err != nil {
		return err
	}

	err = m.s.GetDeviceStates(ctx, &models.GetDeviceStatesInput{Mac: m.mac})
	m.lastUDP = time.Now()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		slog.ErrorContext(ctx, "failed to confirm AC States after command",
			slog.Any("err", err),
			slog.String("device", m.mac))
		m.onGetFailure(ctx)
		return m.sleepBackoff(ctx)
	}

	m.onGetSuccess(ctx)
	pollTicker.Reset(m.pollInterval)
	m.wakeIfPending(ctx)
	return nil
}

func (m *deviceMonitor) readPending(ctx context.Context) (*pendingCommands, error) {
	message, err := m.s.cache.ReadMqttMessage(ctx, &modelsRepo.ReadMqttMessageInput{Mac: m.mac})
	if err != nil {
		return nil, err
	}

	pending := &pendingCommands{}
	if message.Mode != nil && !message.Mode.UpdatedAt.Equal(m.modeUpdatedTime) {
		pending.mode = &message.Mode.Mode
		pending.modeAt = message.Mode.UpdatedAt
		pending.anyPending = true
	}
	if message.FanMode != nil && !message.FanMode.UpdatedAt.Equal(m.fanModeUpdatedTime) {
		pending.fanMode = &message.FanMode.FanMode
		pending.fanAt = message.FanMode.UpdatedAt
		pending.anyPending = true
	}
	if message.SwingMode != nil && !message.SwingMode.UpdatedAt.Equal(m.swingModeUpdatedTime) {
		pending.swingMode = &message.SwingMode.SwingMode
		pending.swingAt = message.SwingMode.UpdatedAt
		pending.anyPending = true
	}
	if message.Temperature != nil && !message.Temperature.UpdatedAt.Equal(m.temperatureUpdatedTime) {
		pending.temperature = &message.Temperature.Temperature
		pending.tempAt = message.Temperature.UpdatedAt
		pending.anyPending = true
	}
	if message.IsDisplayOn != nil && !message.IsDisplayOn.UpdatedAt.Equal(m.isDisplayOnUpdatedTime) {
		pending.isDisplayOn = &message.IsDisplayOn.IsDisplayOn
		pending.displayAt = message.IsDisplayOn.UpdatedAt
		pending.anyPending = true
	}
	if message.IsMildewOn != nil && !message.IsMildewOn.UpdatedAt.Equal(m.isMildewOnUpdatedTime) {
		pending.isMildewOn = &message.IsMildewOn.IsMildewOn
		pending.mildewAt = message.IsMildewOn.UpdatedAt
		pending.anyPending = true
	}
	if message.IsCleanOn != nil && !message.IsCleanOn.UpdatedAt.Equal(m.isCleanOnUpdatedTime) {
		pending.isCleanOn = &message.IsCleanOn.IsCleanOn
		pending.cleanAt = message.IsCleanOn.UpdatedAt
		pending.anyPending = true
	}
	if message.IsHealthOn != nil && !message.IsHealthOn.UpdatedAt.Equal(m.isHealthOnUpdatedTime) {
		pending.isHealthOn = &message.IsHealthOn.IsHealthOn
		pending.healthAt = message.IsHealthOn.UpdatedAt
		pending.anyPending = true
	}
	return pending, nil
}

func (m *deviceMonitor) rememberApplied(pending *pendingCommands) {
	if pending.mode != nil {
		m.modeUpdatedTime = pending.modeAt
	}
	if pending.fanMode != nil {
		m.fanModeUpdatedTime = pending.fanAt
	}
	if pending.swingMode != nil {
		m.swingModeUpdatedTime = pending.swingAt
	}
	if pending.temperature != nil {
		m.temperatureUpdatedTime = pending.tempAt
	}
	if pending.isDisplayOn != nil {
		m.isDisplayOnUpdatedTime = pending.displayAt
	}
	if pending.isMildewOn != nil {
		m.isMildewOnUpdatedTime = pending.mildewAt
	}
	if pending.isCleanOn != nil {
		m.isCleanOnUpdatedTime = pending.cleanAt
	}
	if pending.isHealthOn != nil {
		m.isHealthOnUpdatedTime = pending.healthAt
	}
}

func (m *deviceMonitor) hasRawStatus(ctx context.Context) bool {
	_, err := m.s.cache.ReadDeviceStatusRaw(ctx, &modelsRepo.ReadDeviceStatusRawInput{Mac: m.mac})
	if err == nil {
		return true
	}
	if !errors.Is(err, modelsRepo.ErrorDeviceStatusRawNotFound) {
		slog.ErrorContext(ctx, "failed to read raw device status",
			slog.Any("err", err),
			slog.String("device", m.mac))
	}
	return false
}

func (m *deviceMonitor) wakeIfPending(ctx context.Context) {
	pending, err := m.readPending(ctx)
	if err != nil || pending == nil || !pending.anyPending {
		return
	}
	m.s.wakeMonitor(m.mac)
}

func (m *deviceMonitor) onGetSuccess(ctx context.Context) {
	m.lastSuccess = time.Now()
	m.consecutiveFails = 0
	m.backoffStep = 0
	m.hasSuccess = true
	slog.DebugContext(ctx, "device poll succeeded",
		slog.String("device", m.mac),
		slog.Time("lastSuccess", m.lastSuccess))
	m.setAvailability(ctx, true)
}

func (m *deviceMonitor) onGetFailure(ctx context.Context) {
	m.consecutiveFails++
	if m.consecutiveFails >= offlineFailCount || (!m.hasSuccess && time.Since(m.startedAt) > m.offlineAfter) {
		m.setAvailability(ctx, false)
	}
}

func (m *deviceMonitor) setAvailability(ctx context.Context, online bool) {
	if m.isOnline == online {
		return
	}

	availability := models.StatusOffline
	if online {
		availability = models.StatusOnline
	}

	err := m.s.UpdateDeviceAvailability(ctx, &models.UpdateDeviceAvailabilityInput{
		Mac:          m.mac,
		Availability: availability,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to update device availability",
			slog.Any("err", err),
			slog.String("device", m.mac),
			slog.String("availability", availability))
		return
	}
	m.isOnline = online
}

func (m *deviceMonitor) waitMinGap(ctx context.Context) error {
	return m.waitBeforeUDP(ctx, minUdpGap)
}

func (m *deviceMonitor) waitBeforeUDP(ctx context.Context, minWait time.Duration) error {
	if minWait < minUdpGap {
		minWait = minUdpGap
	}
	if m.lastUDP.IsZero() {
		return nil
	}
	wait := minWait - time.Since(m.lastUDP)
	if wait <= 0 {
		return nil
	}
	return sleepCtx(ctx, wait)
}

func (m *deviceMonitor) sleepBackoff(ctx context.Context) error {
	d := minBackoff << m.backoffStep
	if d <= 0 || d > m.backoffCap {
		d = m.backoffCap
	}
	if m.backoffStep < 16 {
		m.backoffStep++
	}
	return sleepCtx(ctx, d)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

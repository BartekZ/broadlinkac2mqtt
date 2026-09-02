package service

import (
	"context"
	"errors"
	"log/slog"

	modelsMqtt "github.com/ArtemVladimirov/broadlinkac2mqtt/app/mqtt/models"
	modelsRepo "github.com/ArtemVladimirov/broadlinkac2mqtt/app/repository/models"
	"github.com/ArtemVladimirov/broadlinkac2mqtt/app/service/models"
	"github.com/ArtemVladimirov/broadlinkac2mqtt/pkg/converter"
	"golang.org/x/sync/errgroup"
)

func (s *service) publishAndCacheState(ctx context.Context, mac string, state *models.DeviceState) error {
	if state == nil {
		return errors.New("device state is empty")
	}

	previous, err := s.cache.ReadDeviceStatusHass(ctx, &modelsRepo.ReadDeviceStatusHassInput{Mac: mac})
	if err != nil {
		switch {
		case errors.Is(err, modelsRepo.ErrorDeviceStatusHassNotFound):
			previous = nil
		default:
			slog.ErrorContext(ctx, "failed to read the device status",
				slog.Any("err", err),
				slog.String("device", mac))
			return err
		}
	}

	readDeviceConfigReturn, err := s.cache.ReadDeviceConfig(ctx, &modelsRepo.ReadDeviceConfigInput{Mac: mac})
	if err != nil {
		slog.ErrorContext(ctx, "failed to read device config",
			slog.Any("err", err),
			slog.String("device", mac))
		return err
	}

	deviceStatusHA := state.Status
	slog.DebugContext(ctx, "The converted current device status",
		slog.String("device", mac),
		slog.Any("status", deviceStatusHA))

	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		if previous == nil || previous.Status.Temperature != deviceStatusHA.Temperature {
			err := s.mqtt.PublishTemperature(gCtx, &modelsMqtt.PublishTemperatureInput{
				Mac:         mac,
				Temperature: converter.Temperature(models.Celsius, readDeviceConfigReturn.Config.TemperatureUnit, deviceStatusHA.Temperature),
			})
			if err != nil {
				slog.ErrorContext(gCtx, "failed to publish the device set temperature",
					slog.Any("err", err),
					slog.String("device", mac))
				return err
			}
		}
		return nil
	})

	g.Go(func() error {
		if previous == nil || previous.Status.Mode != deviceStatusHA.Mode {
			err := s.mqtt.PublishMode(gCtx, &modelsMqtt.PublishModeInput{
				Mac:  mac,
				Mode: deviceStatusHA.Mode,
			})
			if err != nil {
				slog.ErrorContext(gCtx, "failed to publish the device mode",
					slog.Any("err", err),
					slog.String("device", mac))
				return err
			}
		}
		return nil
	})

	g.Go(func() error {
		if previous == nil || previous.Status.FanMode != deviceStatusHA.FanMode {
			err := s.mqtt.PublishFanMode(gCtx, &modelsMqtt.PublishFanModeInput{
				Mac:     mac,
				FanMode: deviceStatusHA.FanMode,
			})
			if err != nil {
				slog.ErrorContext(gCtx, "failed to publish the device fan mode",
					slog.Any("err", err),
					slog.String("device", mac))
				return err
			}
		}
		return nil
	})

	g.Go(func() error {
		if deviceStatusHA.SwingMode == "" {
			slog.WarnContext(gCtx, "skip publishing empty swing mode", slog.String("device", mac))
			return nil
		}
		if previous == nil || previous.Status.SwingMode != deviceStatusHA.SwingMode {
			err := s.mqtt.PublishSwingMode(gCtx, &modelsMqtt.PublishSwingModeInput{
				Mac:       mac,
				SwingMode: deviceStatusHA.SwingMode,
			})
			if err != nil {
				slog.ErrorContext(gCtx, "failed to publish the device swing mode",
					slog.Any("err", err),
					slog.String("device", mac))
				return err
			}
		}
		return nil
	})

	g.Go(func() error {
		if previous == nil || previous.Status.DisplaySwitch != deviceStatusHA.DisplaySwitch {
			err := s.mqtt.PublishDisplaySwitch(gCtx, &modelsMqtt.PublishDisplaySwitchInput{
				Mac:    mac,
				Status: deviceStatusHA.DisplaySwitch,
			})
			if err != nil {
				slog.ErrorContext(gCtx, "failed to publish the display switch status",
					slog.Any("err", err),
					slog.String("device", mac))
				return err
			}
		}
		return nil
	})

	g.Go(func() error {
		if previous == nil || previous.Status.MildewSwitch != deviceStatusHA.MildewSwitch {
			err := s.mqtt.PublishMildewSwitch(gCtx, &modelsMqtt.PublishMildewSwitchInput{
				Mac:    mac,
				Status: deviceStatusHA.MildewSwitch,
			})
			if err != nil {
				slog.ErrorContext(gCtx, "failed to publish the mildew switch status",
					slog.Any("err", err),
					slog.String("device", mac))
				return err
			}
		}
		return nil
	})

	g.Go(func() error {
		if previous == nil || previous.Status.CleanSwitch != deviceStatusHA.CleanSwitch {
			err := s.mqtt.PublishCleanSwitch(gCtx, &modelsMqtt.PublishCleanSwitchInput{
				Mac:    mac,
				Status: deviceStatusHA.CleanSwitch,
			})
			if err != nil {
				slog.ErrorContext(gCtx, "failed to publish the clean switch status",
					slog.Any("err", err),
					slog.String("device", mac))
				return err
			}
		}
		return nil
	})

	g.Go(func() error {
		if previous == nil || previous.Status.HealthSwitch != deviceStatusHA.HealthSwitch {
			err := s.mqtt.PublishHealthSwitch(gCtx, &modelsMqtt.PublishHealthSwitchInput{
				Mac:    mac,
				Status: deviceStatusHA.HealthSwitch,
			})
			if err != nil {
				slog.ErrorContext(gCtx, "failed to publish the health switch status",
					slog.Any("err", err),
					slog.String("device", mac))
				return err
			}
		}
		return nil
	})

	if err = g.Wait(); err != nil {
		return err
	}

	if state.AmbientTemp != nil {
		if err = s.publishAmbientIfChanged(ctx, mac, *state.AmbientTemp); err != nil {
			return err
		}
	}

	if state.Raw != nil {
		err = s.cache.UpsertDeviceStatusRaw(ctx, &modelsRepo.UpsertDeviceStatusRawInput{
			Mac:    mac,
			Status: modelsRepo.DeviceStatusRaw(*state.Raw),
		})
		if err != nil {
			slog.ErrorContext(ctx, "failed to upsert the raw device status",
				slog.Any("err", err),
				slog.String("device", mac))
			return err
		}
	}

	err = s.cache.UpsertDeviceStatusHass(ctx, &modelsRepo.UpsertDeviceStatusHassInput{
		Mac:    mac,
		Status: modelsRepo.DeviceStatusHass(deviceStatusHA),
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to upsert the device status",
			slog.Any("err", err),
			slog.String("device", mac))
		return err
	}

	return nil
}

func (s *service) publishAmbientIfChanged(ctx context.Context, mac string, ambientTemp float32) error {
	readAmbientTempReturn, err := s.cache.ReadAmbientTemp(ctx, &modelsRepo.ReadAmbientTempInput{Mac: mac})
	if err != nil && !errors.Is(err, modelsRepo.ErrorDeviceStatusAmbientTempNotFound) {
		slog.ErrorContext(ctx, "failed to read the ambient temperature",
			slog.Any("err", err),
			slog.String("device", mac))
		return err
	}

	if readAmbientTempReturn != nil {
		if readAmbientTempReturn.Temperature-ambientTemp > 4 || ambientTemp-readAmbientTempReturn.Temperature > 4 {
			slog.ErrorContext(ctx, "failed to read the ambient temperature", slog.String("device", mac))
			return models.ErrorInvalidParameterTemperature
		}
	}

	slog.DebugContext(ctx, "Ambient temperature",
		slog.Any("ambientTemp", ambientTemp),
		slog.String("device", mac))

	if readAmbientTempReturn == nil || readAmbientTempReturn.Temperature != ambientTemp {
		readDeviceConfigReturn, err := s.cache.ReadDeviceConfig(ctx, &modelsRepo.ReadDeviceConfigInput{Mac: mac})
		if err != nil {
			slog.ErrorContext(ctx, "failed to read device config",
				slog.Any("err", err),
				slog.String("device", mac))
			return err
		}

		err = s.mqtt.PublishAmbientTemp(ctx, &modelsMqtt.PublishAmbientTempInput{
			Mac:         mac,
			Temperature: converter.Temperature(models.Celsius, readDeviceConfigReturn.Config.TemperatureUnit, ambientTemp),
		})
		if err != nil {
			slog.ErrorContext(ctx, "failed to publish ambient temperature",
				slog.Any("err", err),
				slog.String("device", mac))
			return err
		}

		err = s.cache.UpsertAmbientTemp(ctx, &modelsRepo.UpsertAmbientTempInput{Temperature: ambientTemp, Mac: mac})
		if err != nil {
			slog.ErrorContext(ctx, "failed to upsert the temperature",
				slog.Any("err", err),
				slog.String("device", mac))
			return err
		}
	}

	return nil
}

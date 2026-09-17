package service

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/ArtemVladimirov/broadlinkac2mqtt/app"
	modelsMqtt "github.com/ArtemVladimirov/broadlinkac2mqtt/app/mqtt/models"
	modelsRepo "github.com/ArtemVladimirov/broadlinkac2mqtt/app/repository/models"
	"github.com/ArtemVladimirov/broadlinkac2mqtt/app/service/models"
	"github.com/ArtemVladimirov/broadlinkac2mqtt/pkg/converter"
)

type service struct {
	updateInterval int
	topicPrefix    string
	mqtt           app.MqttPublisher
	cache          app.Cache
	localBackend   app.DeviceBackend
	cloudBackend   app.DeviceBackend
	commandNotify  sync.Map // mac -> chan struct{}
}

func NewService(topicPrefix string, updateInterval int, mqtt app.MqttPublisher, webClient app.WebClient, cache app.Cache, cloudBackend app.DeviceBackend) app.Service {
	return &service{
		topicPrefix:    topicPrefix,
		updateInterval: updateInterval,
		mqtt:           mqtt,
		cache:          cache,
		localBackend:   NewLocalBackend(webClient, cache),
		cloudBackend:   cloudBackend,
	}
}

func (s *service) backendFor(ctx context.Context, mac string) (app.DeviceBackend, error) {
	readDeviceConfigReturn, err := s.cache.ReadDeviceConfig(ctx, &modelsRepo.ReadDeviceConfigInput{Mac: mac})
	if err != nil {
		return nil, err
	}
	if readDeviceConfigReturn.Config.IsCloud() {
		if s.cloudBackend == nil {
			return nil, errors.New("cloud backend is not configured")
		}
		return s.cloudBackend, nil
	}
	return s.localBackend, nil
}

func (s *service) capabilities(ctx context.Context, mac string) models.DeviceCapabilities {
	backend, err := s.backendFor(ctx, mac)
	if err != nil {
		return models.DefaultLocalCapabilities()
	}
	return backend.Capabilities(ctx, mac)
}

func (s *service) CreateDevice(ctx context.Context, input *models.CreateDeviceInput) error {
	if input.Config.Backend == "" {
		input.Config.Backend = models.BackendLocal
	}

	err := s.cache.UpsertDeviceConfig(ctx, &modelsRepo.UpsertDeviceConfigInput{
		Config: modelsRepo.DeviceConfig(input.Config),
	})
	if err != nil {
		return err
	}

	if input.Config.IsCloud() {
		return nil
	}

	local, ok := s.localBackend.(*localBackend)
	if !ok {
		return errors.New("local backend is not available")
	}
	return local.Create(ctx, input.Config.Mac)
}

func (s *service) AuthDevice(ctx context.Context, input *models.AuthDeviceInput) error {
	backend, err := s.backendFor(ctx, input.Mac)
	if err != nil {
		slog.ErrorContext(ctx, "failed to select device backend", slog.Any("err", err), slog.String("device", input.Mac))
		return err
	}
	return backend.Authenticate(ctx, input.Mac)
}

func (s *service) GetDeviceStates(ctx context.Context, input *models.GetDeviceStatesInput) error {
	backend, err := s.backendFor(ctx, input.Mac)
	if err != nil {
		slog.ErrorContext(ctx, "failed to select device backend", slog.Any("err", err), slog.String("device", input.Mac))
		return err
	}

	state, err := backend.ReadState(ctx, input.Mac)
	if err != nil {
		return err
	}

	return s.publishAndCacheState(ctx, input.Mac, state)
}

func (s *service) getDeviceAmbientTemperature(ctx context.Context, input *models.GetDeviceAmbientTemperatureInput) error {
	backend, err := s.backendFor(ctx, input.Mac)
	if err != nil {
		slog.ErrorContext(ctx, "failed to select device backend", slog.Any("err", err), slog.String("device", input.Mac))
		return err
	}

	ambientTemp, err := backend.ReadAmbient(ctx, input.Mac)
	if err != nil {
		return err
	}
	if ambientTemp == nil {
		return nil
	}

	return s.publishAmbientIfChanged(ctx, input.Mac, *ambientTemp)
}

func (s *service) UpdateDeviceStates(ctx context.Context, input *models.UpdateDeviceStatesInput) error {
	backend, err := s.backendFor(ctx, input.Mac)
	if err != nil {
		slog.ErrorContext(ctx, "failed to select device backend", slog.Any("err", err), slog.String("device", input.Mac))
		return err
	}
	return backend.ApplyState(ctx, input.Mac, input)
}

func (s *service) PublishDiscoveryTopic(ctx context.Context, input *models.PublishDiscoveryTopicInput) error {
	prefix := s.topicPrefix + "/" + input.Device.Mac
	caps := s.capabilities(ctx, input.Device.Mac)

	device := modelsMqtt.DiscoveryTopicDevice{
		Model: "AirCon",
		Mf:    "broadlink",
		Sw:    "v1.6.0",
		Ids:   input.Device.Mac,
		Name:  input.Device.Name,
	}
	if input.Device.IsCloud() {
		device.Mf = "aux"
		device.Model = "AirCon Cloud"
	}

	availability := modelsMqtt.DiscoveryTopicAvailability{
		PayloadAvailable:    models.StatusOnline,
		PayloadNotAvailable: models.StatusOffline,
		Topic:               prefix + "/availability/value",
	}

	err := s.mqtt.PublishClimateDiscoveryTopic(ctx, modelsMqtt.PublishClimateDiscoveryTopicInput{
		Topic: modelsMqtt.ClimateDiscoveryTopic{
			FanModeCommandTopic:     prefix + "/fan_mode/set",
			FanModes:                caps.FanModes,
			FanModeStateTopic:       prefix + "/fan_mode/value",
			ModeCommandTopic:        prefix + "/mode/set",
			ModeStateTopic:          prefix + "/mode/value",
			Modes:                   caps.Modes,
			SwingModeCommandTopic:   prefix + "/swing_mode/set",
			SwingModeStateTopic:     prefix + "/swing_mode/value",
			SwingModes:              caps.SwingModes,
			MinTemp:                 16,
			MaxTemp:                 32,
			TempStep:                0.5,
			TemperatureStateTopic:   prefix + "/temp/value",
			TemperatureCommandTopic: prefix + "/temp/set",
			Precision:               0.1,
			Device:                  device,
			UniqueId:                input.Device.Mac + "_ac",
			Availability:            availability,
			CurrentTemperatureTopic: prefix + "/current_temp/value",
			Name:                    nil,
			Icon:                    "mdi:air-conditioner",
			TemperatureUnit:         input.Device.TemperatureUnit,
		},
	})
	if err != nil {
		return err
	}

	if caps.DisplaySwitch {
		err = s.mqtt.PublishSwitchDiscoveryTopic(ctx, modelsMqtt.PublishSwitchDiscoveryTopicInput{
			Topic: modelsMqtt.SwitchDiscoveryTopic{
				Device:       device,
				Name:         "Screen",
				UniqueId:     input.Device.Mac + "_screen",
				StateTopic:   prefix + "/display/switch/value",
				CommandTopic: prefix + "/display/switch/set",
				Availability: availability,
				Icon:         "mdi:tablet-dashboard",
			},
		})
		if err != nil {
			return err
		}
	}

	if caps.MildewSwitch {
		err = s.mqtt.PublishSwitchDiscoveryTopic(ctx, modelsMqtt.PublishSwitchDiscoveryTopicInput{
			Topic: modelsMqtt.SwitchDiscoveryTopic{
				Device:       device,
				Name:         "Anti-mold",
				UniqueId:     input.Device.Mac + "_mildew",
				StateTopic:   prefix + "/mildew/switch/value",
				CommandTopic: prefix + "/mildew/switch/set",
				Availability: availability,
				Icon:         "mdi:water-off",
			},
		})
		if err != nil {
			return err
		}
	}

	if caps.CleanSwitch {
		err = s.mqtt.PublishSwitchDiscoveryTopic(ctx, modelsMqtt.PublishSwitchDiscoveryTopicInput{
			Topic: modelsMqtt.SwitchDiscoveryTopic{
				Device:       device,
				Name:         "Clean",
				UniqueId:     input.Device.Mac + "_clean",
				StateTopic:   prefix + "/clean/switch/value",
				CommandTopic: prefix + "/clean/switch/set",
				Availability: availability,
				Icon:         "mdi:shimmer",
			},
		})
		if err != nil {
			return err
		}
	}

	if caps.HealthSwitch {
		enabledByDefaultFalse := false
		err = s.mqtt.PublishSwitchDiscoveryTopic(ctx, modelsMqtt.PublishSwitchDiscoveryTopicInput{
			Topic: modelsMqtt.SwitchDiscoveryTopic{
				Device:           device,
				Name:             "Health",
				UniqueId:         input.Device.Mac + "_health",
				StateTopic:       prefix + "/health/switch/value",
				CommandTopic:     prefix + "/health/switch/set",
				Availability:     availability,
				Icon:             "mdi:air-purifier",
				EnabledByDefault: &enabledByDefaultFalse,
			},
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *service) UpdateFanMode(ctx context.Context, input *models.UpdateFanModeInput) error {
	err := input.Validate()
	if err != nil {
		slog.ErrorContext(ctx, "input data is not valid",
			slog.Any("err", err),
			slog.String("device", input.Mac),
			slog.Any("input", input))
		return err
	}
	if !s.capabilities(ctx, input.Mac).HasFanMode(input.FanMode) {
		return models.ErrorInvalidParameterFanMode
	}

	err = s.cache.UpsertMqttFanModeMessage(ctx, &modelsRepo.UpsertMqttFanModeMessageInput{
		Mac: input.Mac,
		FanMode: modelsRepo.MqttFanModeMessage{
			UpdatedAt: time.Now(),
			FanMode:   input.FanMode,
		},
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to save mqtt message to cache storage",
			slog.Any("err", err),
			slog.String("device", input.Mac))
		return err
	}
	s.wakeMonitor(input.Mac)

	err = s.mqtt.PublishFanMode(ctx, &modelsMqtt.PublishFanModeInput{
		Mac:     input.Mac,
		FanMode: input.FanMode,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to publish fan mode to mqtt",
			slog.Any("err", err),
			slog.String("device", input.Mac))
		return err
	}

	return nil
}

func (s *service) UpdateMode(ctx context.Context, input *models.UpdateModeInput) error {
	err := input.Validate()
	if err != nil {
		slog.ErrorContext(ctx, "input data is not valid",
			slog.Any("err", err),
			slog.String("device", input.Mac),
			slog.Any("input", input))
		return err
	}
	if !s.capabilities(ctx, input.Mac).HasMode(input.Mode) {
		return models.ErrorInvalidParameterMode
	}

	err = s.cache.UpsertMqttModeMessage(ctx, &modelsRepo.UpsertMqttModeMessageInput{
		Mac: input.Mac,
		Mode: modelsRepo.MqttModeMessage{
			UpdatedAt: time.Now(),
			Mode:      input.Mode,
		},
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to save mqtt message to cache storage",
			slog.Any("err", err),
			slog.String("device", input.Mac))
		return err
	}
	s.wakeMonitor(input.Mac)

	err = s.mqtt.PublishMode(ctx, &modelsMqtt.PublishModeInput{
		Mac:  input.Mac,
		Mode: input.Mode,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to publish mode to mqtt",
			slog.Any("err", err),
			slog.String("device", input.Mac))
		return err
	}

	return nil
}

func (s *service) UpdateSwingMode(ctx context.Context, input *models.UpdateSwingModeInput) error {
	err := input.Validate()
	if err != nil {
		slog.ErrorContext(ctx, "input data is not valid",
			slog.Any("err", err),
			slog.String("device", input.Mac),
			slog.Any("input", input))
		return err
	}
	if !s.capabilities(ctx, input.Mac).HasSwingMode(input.SwingMode) {
		return models.ErrorInvalidParameterSwingMode
	}

	err = s.cache.UpsertMqttSwingModeMessage(ctx, &modelsRepo.UpsertMqttSwingModeMessageInput{
		Mac: input.Mac,
		SwingMode: modelsRepo.MqttSwingModeMessage{
			UpdatedAt: time.Now(),
			SwingMode: input.SwingMode,
		},
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to save mqtt message to cache storage",
			slog.Any("err", err),
			slog.String("device", input.Mac))
		return err
	}
	s.wakeMonitor(input.Mac)

	err = s.mqtt.PublishSwingMode(ctx, &modelsMqtt.PublishSwingModeInput{
		Mac:       input.Mac,
		SwingMode: input.SwingMode,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to publish swing mode to mqtt",
			slog.Any("err", err),
			slog.String("device", input.Mac))
		return err
	}

	return nil
}

func (s *service) UpdateTemperature(ctx context.Context, input *models.UpdateTemperatureInput) error {
	readDeviceConfigReturn, err := s.cache.ReadDeviceConfig(ctx, &modelsRepo.ReadDeviceConfigInput{Mac: input.Mac})
	if err != nil {
		slog.ErrorContext(ctx, "failed to read device config",
			slog.Any("err", err),
			slog.String("device", input.Mac))
		return err
	}

	input.Temperature = converter.Temperature(readDeviceConfigReturn.Config.TemperatureUnit, models.Celsius, input.Temperature)
	err = input.Validate()
	if err != nil {
		slog.ErrorContext(ctx, "input data is not valid",
			slog.Any("err", err),
			slog.String("device", input.Mac),
			slog.Any("input", input))
		return err
	}

	err = s.cache.UpsertMqttTemperatureMessage(ctx, &modelsRepo.UpsertMqttTemperatureMessageInput{
		Mac: input.Mac,
		Temperature: modelsRepo.MqttTemperatureMessage{
			UpdatedAt:   time.Now(),
			Temperature: input.Temperature,
		},
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to save mqtt message to cache storage",
			slog.Any("err", err),
			slog.String("device", input.Mac))
		return err
	}
	s.wakeMonitor(input.Mac)

	return nil
}

func (s *service) UpdateDisplaySwitch(ctx context.Context, input *models.UpdateDisplaySwitchInput) error {
	err := input.Validate()
	if err != nil {
		slog.ErrorContext(ctx, "input data is not valid",
			slog.Any("err", err),
			slog.String("device", input.Mac),
			slog.Any("input", input))
		return err
	}
	if !s.capabilities(ctx, input.Mac).DisplaySwitch {
		return models.ErrorInvalidParameterDisplayStatus
	}

	err = s.cache.UpsertMqttDisplaySwitchMessage(ctx, &modelsRepo.UpsertMqttDisplaySwitchMessageInput{
		Mac: input.Mac,
		DisplaySwitch: modelsRepo.MqttDisplaySwitchMessage{
			UpdatedAt:   time.Now(),
			IsDisplayOn: input.Status == "ON",
		},
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to save mqtt message to cache storage",
			slog.Any("err", err),
			slog.String("device", input.Mac))
		return err
	}
	s.wakeMonitor(input.Mac)

	return nil
}

func (s *service) UpdateMildewSwitch(ctx context.Context, input *models.UpdateMildewSwitchInput) error {
	err := input.Validate()
	if err != nil {
		slog.ErrorContext(ctx, "input data is not valid",
			slog.Any("err", err),
			slog.String("device", input.Mac),
			slog.Any("input", input))
		return err
	}
	if !s.capabilities(ctx, input.Mac).MildewSwitch {
		return models.ErrorInvalidParameterMildewStatus
	}

	err = s.cache.UpsertMqttMildewSwitchMessage(ctx, &modelsRepo.UpsertMqttMildewSwitchMessageInput{
		Mac: input.Mac,
		MildewSwitch: modelsRepo.MqttMildewSwitchMessage{
			UpdatedAt:  time.Now(),
			IsMildewOn: input.Status == "ON",
		},
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to save mqtt message to cache storage",
			slog.Any("err", err),
			slog.String("device", input.Mac))
		return err
	}
	s.wakeMonitor(input.Mac)

	return nil
}

func (s *service) UpdateCleanSwitch(ctx context.Context, input *models.UpdateCleanSwitchInput) error {
	err := input.Validate()
	if err != nil {
		slog.ErrorContext(ctx, "input data is not valid",
			slog.Any("err", err),
			slog.String("device", input.Mac),
			slog.Any("input", input))
		return err
	}
	if !s.capabilities(ctx, input.Mac).CleanSwitch {
		return models.ErrorInvalidParameterCleanStatus
	}

	err = s.cache.UpsertMqttCleanSwitchMessage(ctx, &modelsRepo.UpsertMqttCleanSwitchMessageInput{
		Mac: input.Mac,
		CleanSwitch: modelsRepo.MqttCleanSwitchMessage{
			UpdatedAt: time.Now(),
			IsCleanOn: input.Status == "ON",
		},
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to save mqtt message to cache storage",
			slog.Any("err", err),
			slog.String("device", input.Mac))
		return err
	}
	s.wakeMonitor(input.Mac)

	return nil
}

func (s *service) UpdateHealthSwitch(ctx context.Context, input *models.UpdateHealthSwitchInput) error {
	err := input.Validate()
	if err != nil {
		slog.ErrorContext(ctx, "input data is not valid",
			slog.Any("err", err),
			slog.String("device", input.Mac),
			slog.Any("input", input))
		return err
	}
	if !s.capabilities(ctx, input.Mac).HealthSwitch {
		return models.ErrorInvalidParameterHealthStatus
	}

	err = s.cache.UpsertMqttHealthSwitchMessage(ctx, &modelsRepo.UpsertMqttHealthSwitchMessageInput{
		Mac: input.Mac,
		HealthSwitch: modelsRepo.MqttHealthSwitchMessage{
			UpdatedAt:  time.Now(),
			IsHealthOn: input.Status == "ON",
		},
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to save mqtt message to cache storage",
			slog.Any("err", err),
			slog.String("device", input.Mac))
		return err
	}
	s.wakeMonitor(input.Mac)

	return nil
}

func (s *service) UpdateDeviceAvailability(ctx context.Context, input *models.UpdateDeviceAvailabilityInput) error {
	err := s.cache.UpsertDeviceAvailability(ctx, &modelsRepo.UpsertDeviceAvailabilityInput{
		Mac:          input.Mac,
		Availability: input.Availability,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to upsert device availability",
			slog.Any("err", err),
			slog.String("device", input.Mac))
		return err
	}

	err = s.mqtt.PublishAvailability(ctx, &modelsMqtt.PublishAvailabilityInput{
		Mac:          input.Mac,
		Availability: input.Availability,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to publish device availability",
			slog.Any("err", err),
			slog.String("device", input.Mac))
		return err
	}

	return nil
}

func (s *service) wakeMonitor(mac string) {
	v, ok := s.commandNotify.Load(mac)
	if !ok {
		return
	}
	ch, ok := v.(chan struct{})
	if !ok {
		return
	}
	select {
	case ch <- struct{}{}:
	default:
	}
}

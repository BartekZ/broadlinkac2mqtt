package service

import (
	"context"
	"log/slog"
	"time"

	modelsMqtt "github.com/ArtemVladimirov/broadlinkac2mqtt/app/mqtt/models"
	modelsRepo "github.com/ArtemVladimirov/broadlinkac2mqtt/app/repository/models"
	"github.com/ArtemVladimirov/broadlinkac2mqtt/app/service/models"
	"github.com/ArtemVladimirov/broadlinkac2mqtt/pkg/converter"
	"golang.org/x/sync/errgroup"
)

func (s *service) PublishStatesOnHomeAssistantRestart(ctx context.Context, input *models.PublishStatesOnHomeAssistantRestartInput) error {
	if input.Status != models.StatusOnline {
		return nil
	}

	readAuthedDevicesReturn, err := s.cache.ReadAuthedDevices(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to read authed devices",
			slog.Any("err", err))
		return err
	}

	eg, gCtx := errgroup.WithContext(ctx)
	for _, mac := range readAuthedDevicesReturn.Macs {
		eg.Go(func() error {
			/////////////////////////////////
			// Read all states and configs //
			/////////////////////////////////

			readDeviceStatusRawInput := &modelsRepo.ReadDeviceStatusRawInput{
				Mac: mac,
			}
			readDeviceStatusRawReturn, err := s.cache.ReadDeviceStatusRaw(gCtx, readDeviceStatusRawInput)
			if err != nil {
				s.logger.ErrorContext(gCtx, "failed to read the device status",
					slog.Any("err", err),
					slog.Any("input", readDeviceStatusRawInput))
				return err
			}

			readDeviceConfigInput := &modelsRepo.ReadDeviceConfigInput{
				Mac: mac,
			}
			readDeviceConfigReturn, err := s.cache.ReadDeviceConfig(gCtx, readDeviceConfigInput)
			if err != nil {
				s.logger.ErrorContext(gCtx, "failed to read device config",
					slog.Any("err", err),
					slog.Any("input", readDeviceConfigInput))
				return err
			}

			hassStatus := models.DeviceStatusRaw(readDeviceStatusRawReturn.Status).ConvertToDeviceStatusHA(readDeviceConfigReturn.Config.InvertDisplay)

			readAmbientTempInput := &modelsRepo.ReadAmbientTempInput{Mac: mac}

			readAmbientTempReturn, err := s.cache.ReadAmbientTemp(gCtx, readAmbientTempInput)
			if err != nil {
				s.logger.ErrorContext(gCtx, "failed to read the ambient temperature",
					slog.Any("err", err),
					slog.Any("input", readAmbientTempInput))
				return err
			}

			readDeviceAvailabilityInput := &modelsRepo.ReadDeviceAvailabilityInput{Mac: mac}

			readDeviceAvailabilityReturn, err := s.cache.ReadDeviceAvailability(gCtx, readDeviceAvailabilityInput)
			if err != nil {
				s.logger.ErrorContext(gCtx, "failed to read the device availability",
					slog.Any("err", err),
					slog.Any("input", readDeviceAvailabilityInput))
				return err
			}

			/////////////////////////////////
			// 		Publish all topics     //
			/////////////////////////////////

			err = s.PublishDiscoveryTopic(gCtx, &models.PublishDiscoveryTopicInput{Device: models.DeviceConfig(readDeviceConfigReturn.Config)})
			if err != nil {
				s.logger.ErrorContext(gCtx, "failed to publish the discovery topic",
					slog.Any("err", err),
					slog.Any("input", readDeviceConfigReturn.Config))
				return err
			}

			time.Sleep(time.Millisecond * 500)

			publishAvailabilityInput := &modelsMqtt.PublishAvailabilityInput{
				Mac:          mac,
				Availability: readDeviceAvailabilityReturn.Availability,
			}
			err = s.mqtt.PublishAvailability(gCtx, publishAvailabilityInput)
			if err != nil {
				s.logger.ErrorContext(gCtx, "failed to publish device availability",
					slog.Any("err", err),
					slog.Any("input", publishAvailabilityInput))
				return err
			}

			// Send  temperature to MQTT
			publishAmbientTempInput := &modelsMqtt.PublishAmbientTempInput{
				Mac:         mac,
				Temperature: converter.Temperature(models.Celsius, readDeviceConfigReturn.Config.TemperatureUnit, readAmbientTempReturn.Temperature),
			}
			err = s.mqtt.PublishAmbientTemp(gCtx, publishAmbientTempInput)
			if err != nil {
				s.logger.ErrorContext(gCtx, "failed to publish ambient temperature",
					slog.Any("err", err),
					slog.Any("input", publishAmbientTempInput))
				return err
			}

			publishTemperatureInput := &modelsMqtt.PublishTemperatureInput{
				Mac:         mac,
				Temperature: converter.Temperature(models.Celsius, readDeviceConfigReturn.Config.TemperatureUnit, readDeviceStatusRawReturn.Status.Temperature),
			}
			err = s.mqtt.PublishTemperature(gCtx, publishTemperatureInput)
			if err != nil {
				s.logger.ErrorContext(gCtx, "failed to publish the device set temperature",
					slog.Any("err", err),
					slog.Any("input", publishTemperatureInput))
				return err
			}

			publishModeInput := &modelsMqtt.PublishModeInput{
				Mac:  mac,
				Mode: hassStatus.Mode,
			}
			err = s.mqtt.PublishMode(gCtx, publishModeInput)
			if err != nil {
				s.logger.ErrorContext(gCtx, "failed to publish the device mode",
					slog.Any("err", err),
					slog.Any("input", publishModeInput))
				return err
			}

			publishFanModeInput := &modelsMqtt.PublishFanModeInput{
				Mac:     mac,
				FanMode: hassStatus.FanMode,
			}
			err = s.mqtt.PublishFanMode(gCtx, publishFanModeInput)
			if err != nil {
				s.logger.ErrorContext(gCtx, "failed to publish the device fan mode",
					slog.Any("err", err),
					slog.Any("input", publishFanModeInput))
				return err
			}

			publishSwingModeInput := &modelsMqtt.PublishSwingModeInput{
				Mac:       mac,
				SwingMode: hassStatus.SwingMode,
			}
			err = s.mqtt.PublishSwingMode(gCtx, publishSwingModeInput)
			if err != nil {
				s.logger.ErrorContext(gCtx, "failed to publish the device swing mode",
					slog.Any("err", err),
					slog.Any("input", publishSwingModeInput))
				return err
			}

			publishDisplaySwitchInput := &modelsMqtt.PublishDisplaySwitchInput{
				Mac:    mac,
				Status: hassStatus.DisplaySwitch,
			}
			err = s.mqtt.PublishDisplaySwitch(gCtx, publishDisplaySwitchInput)
			if err != nil {
				s.logger.ErrorContext(gCtx, "failed to publish the display switch status",
					slog.Any("err", err),
					slog.Any("input", publishDisplaySwitchInput))
				return err
			}

			publishMildewSwitchInput := &modelsMqtt.PublishMildewSwitchInput{
				Mac:    mac,
				Status: hassStatus.MildewSwitch,
			}
			err = s.mqtt.PublishMildewSwitch(gCtx, publishMildewSwitchInput)
			if err != nil {
				s.logger.ErrorContext(gCtx, "failed to publish the mildew switch status",
					slog.Any("err", err),
					slog.Any("input", publishMildewSwitchInput))
				return err
			}

			publishCleanSwitchInput := &modelsMqtt.PublishCleanSwitchInput{
				Mac:    mac,
				Status: hassStatus.CleanSwitch,
			}
			err = s.mqtt.PublishCleanSwitch(gCtx, publishCleanSwitchInput)
			if err != nil {
				s.logger.ErrorContext(gCtx, "failed to publish the clean switch status",
					slog.Any("err", err),
					slog.Any("input", publishCleanSwitchInput))
				return err
			}

			publishHealthSwitchInput := &modelsMqtt.PublishHealthSwitchInput{
				Mac:    mac,
				Status: hassStatus.HealthSwitch,
			}
			err = s.mqtt.PublishHealthSwitch(gCtx, publishHealthSwitchInput)
			if err != nil {
				s.logger.ErrorContext(gCtx, "failed to publish the health switch status",
					slog.Any("err", err),
					slog.Any("input", publishHealthSwitchInput))
				return err
			}
			return nil
		})
	}

	return eg.Wait()
}

package service

import (
	"context"
	"errors"
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
		slog.ErrorContext(ctx, "failed to read authed devices",
			slog.Any("err", err))
		return err
	}

	eg, gCtx := errgroup.WithContext(ctx)
	for _, mac := range readAuthedDevicesReturn.Macs {
		eg.Go(func() error {
			hassStatus, err := s.readHassStatus(gCtx, mac)
			if err != nil {
				slog.ErrorContext(gCtx, "failed to read the device status",
					slog.Any("err", err),
					slog.String("device", mac))
				return err
			}

			readDeviceConfigReturn, err := s.cache.ReadDeviceConfig(gCtx, &modelsRepo.ReadDeviceConfigInput{Mac: mac})
			if err != nil {
				slog.ErrorContext(gCtx, "failed to read device config",
					slog.Any("err", err),
					slog.String("device", mac))
				return err
			}

			readAmbientTempReturn, err := s.cache.ReadAmbientTemp(gCtx, &modelsRepo.ReadAmbientTempInput{Mac: mac})
			if err != nil && !errors.Is(err, modelsRepo.ErrorDeviceStatusAmbientTempNotFound) {
				slog.ErrorContext(gCtx, "failed to read the ambient temperature",
					slog.Any("err", err),
					slog.String("device", mac))
				return err
			}

			readDeviceAvailabilityReturn, err := s.cache.ReadDeviceAvailability(gCtx, &modelsRepo.ReadDeviceAvailabilityInput{Mac: mac})
			if err != nil {
				slog.ErrorContext(gCtx, "failed to read the device availability",
					slog.Any("err", err),
					slog.String("device", mac))
				return err
			}

			err = s.PublishDiscoveryTopic(gCtx, &models.PublishDiscoveryTopicInput{Device: models.DeviceConfig(readDeviceConfigReturn.Config)})
			if err != nil {
				slog.ErrorContext(gCtx, "failed to publish the discovery topic",
					slog.Any("err", err),
					slog.Any("input", readDeviceConfigReturn.Config))
				return err
			}

			if err := sleepCtx(gCtx, 500*time.Millisecond); err != nil {
				return err
			}

			err = s.mqtt.PublishAvailability(gCtx, &modelsMqtt.PublishAvailabilityInput{
				Mac:          mac,
				Availability: readDeviceAvailabilityReturn.Availability,
			})
			if err != nil {
				slog.ErrorContext(gCtx, "failed to publish device availability",
					slog.Any("err", err),
					slog.String("device", mac))
				return err
			}

			if readAmbientTempReturn != nil {
				err = s.mqtt.PublishAmbientTemp(gCtx, &modelsMqtt.PublishAmbientTempInput{
					Mac:         mac,
					Temperature: converter.Temperature(models.Celsius, readDeviceConfigReturn.Config.TemperatureUnit, readAmbientTempReturn.Temperature),
				})
				if err != nil {
					slog.ErrorContext(gCtx, "failed to publish ambient temperature",
						slog.Any("err", err),
						slog.String("device", mac))
					return err
				}
			}

			err = s.mqtt.PublishTemperature(gCtx, &modelsMqtt.PublishTemperatureInput{
				Mac:         mac,
				Temperature: converter.Temperature(models.Celsius, readDeviceConfigReturn.Config.TemperatureUnit, hassStatus.Temperature),
			})
			if err != nil {
				slog.ErrorContext(gCtx, "failed to publish the device set temperature",
					slog.Any("err", err),
					slog.String("device", mac))
				return err
			}

			err = s.mqtt.PublishMode(gCtx, &modelsMqtt.PublishModeInput{
				Mac:  mac,
				Mode: hassStatus.Mode,
			})
			if err != nil {
				slog.ErrorContext(gCtx, "failed to publish the device mode",
					slog.Any("err", err),
					slog.String("device", mac))
				return err
			}

			err = s.mqtt.PublishFanMode(gCtx, &modelsMqtt.PublishFanModeInput{
				Mac:     mac,
				FanMode: hassStatus.FanMode,
			})
			if err != nil {
				slog.ErrorContext(gCtx, "failed to publish the device fan mode",
					slog.Any("err", err),
					slog.String("device", mac))
				return err
			}

			if hassStatus.SwingMode != "" {
				err = s.mqtt.PublishSwingMode(gCtx, &modelsMqtt.PublishSwingModeInput{
					Mac:       mac,
					SwingMode: hassStatus.SwingMode,
				})
				if err != nil {
					slog.ErrorContext(gCtx, "failed to publish the device swing mode",
						slog.Any("err", err),
						slog.String("device", mac))
					return err
				}
			}

			err = s.mqtt.PublishDisplaySwitch(gCtx, &modelsMqtt.PublishDisplaySwitchInput{
				Mac:    mac,
				Status: hassStatus.DisplaySwitch,
			})
			if err != nil {
				slog.ErrorContext(gCtx, "failed to publish the display switch status",
					slog.Any("err", err),
					slog.String("device", mac))
				return err
			}

			err = s.mqtt.PublishMildewSwitch(gCtx, &modelsMqtt.PublishMildewSwitchInput{
				Mac:    mac,
				Status: hassStatus.MildewSwitch,
			})
			if err != nil {
				slog.ErrorContext(gCtx, "failed to publish the mildew switch status",
					slog.Any("err", err),
					slog.String("device", mac))
				return err
			}

			err = s.mqtt.PublishCleanSwitch(gCtx, &modelsMqtt.PublishCleanSwitchInput{
				Mac:    mac,
				Status: hassStatus.CleanSwitch,
			})
			if err != nil {
				slog.ErrorContext(gCtx, "failed to publish the clean switch status",
					slog.Any("err", err),
					slog.String("device", mac))
				return err
			}

			err = s.mqtt.PublishHealthSwitch(gCtx, &modelsMqtt.PublishHealthSwitchInput{
				Mac:    mac,
				Status: hassStatus.HealthSwitch,
			})
			if err != nil {
				slog.ErrorContext(gCtx, "failed to publish the health switch status",
					slog.Any("err", err),
					slog.String("device", mac))
				return err
			}
			return nil
		})
	}

	return eg.Wait()
}

func (s *service) readHassStatus(ctx context.Context, mac string) (models.DeviceStatusHass, error) {
	hassReturn, err := s.cache.ReadDeviceStatusHass(ctx, &modelsRepo.ReadDeviceStatusHassInput{Mac: mac})
	if err == nil {
		return models.DeviceStatusHass(hassReturn.Status), nil
	}
	if !errors.Is(err, modelsRepo.ErrorDeviceStatusHassNotFound) {
		return models.DeviceStatusHass{}, err
	}

	rawReturn, err := s.cache.ReadDeviceStatusRaw(ctx, &modelsRepo.ReadDeviceStatusRawInput{Mac: mac})
	if err != nil {
		return models.DeviceStatusHass{}, err
	}

	readDeviceConfigReturn, err := s.cache.ReadDeviceConfig(ctx, &modelsRepo.ReadDeviceConfigInput{Mac: mac})
	if err != nil {
		return models.DeviceStatusHass{}, err
	}

	return models.DeviceStatusRaw(rawReturn.Status).ConvertToDeviceStatusHA(readDeviceConfigReturn.Config.InvertDisplay), nil
}

package main

import (
	"context"
	"log/slog"
	"strings"

	"github.com/ArtemVladimirov/broadlinkac2mqtt/app/auxcloud"
	workspaceServiceModels "github.com/ArtemVladimirov/broadlinkac2mqtt/app/service/models"
	"github.com/ArtemVladimirov/broadlinkac2mqtt/config"
)

func localDeviceConfigs(cfgDevices []config.Devices) ([]workspaceServiceModels.DeviceConfig, error) {
	devices := make([]workspaceServiceModels.DeviceConfig, 0, len(cfgDevices))
	for _, device := range cfgDevices {
		if device.Mac == "" && device.Ip == "" && device.DeviceID == "" {
			continue
		}

		if len(device.TemperatureUnit) == 0 {
			device.TemperatureUnit = "C"
		}

		backend := strings.ToLower(strings.TrimSpace(device.Transport))
		if backend == "" {
			backend = workspaceServiceModels.BackendLocal
		}

		dev := workspaceServiceModels.DeviceConfig{
			Ip:              device.Ip,
			Mac:             strings.ToLower(device.Mac),
			Name:            device.Name,
			Port:            device.Port,
			TemperatureUnit: strings.ToUpper(device.TemperatureUnit),
			InvertDisplay:   device.InvertDisplay,
			Backend:         backend,
			CloudEndpointID: device.DeviceID,
		}

		err := dev.Validate()
		if err != nil {
			slog.Error("device config is incorrect", slog.String("device", device.Mac), slog.Any("err", err))
			return nil, err
		}

		devices = append(devices, dev)
	}
	return devices, nil
}

func loadDevices(ctx context.Context, cfg *config.Config, cloudClient *auxcloud.Client) ([]workspaceServiceModels.DeviceConfig, error) {
	local, err := localDeviceConfigs(cfg.Devices)
	if err != nil {
		return nil, err
	}

	if cloudClient == nil || !cfg.Cloud.Enabled || !cfg.Cloud.AutoDiscoverEnabled() {
		return local, nil
	}

	discovered, err := cloudClient.DiscoverDevices(ctx)
	if err != nil {
		return nil, err
	}

	unit := strings.ToUpper(cfg.Cloud.TemperatureUnit)
	if unit == "" {
		unit = "C"
	}

	cloud := auxcloud.DeviceConfigsFromDiscovery(discovered, cfg.Cloud.HiddenDevices, unit)
	merged := auxcloud.MergeDevices(local, cloud)
	slog.InfoContext(ctx, "aux cloud devices loaded",
		slog.Int("discovered", len(discovered)),
		slog.Int("added", len(merged)-len(local)))
	return merged, nil
}

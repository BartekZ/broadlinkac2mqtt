package auxcloud

import (
	"strings"

	"github.com/ArtemVladimirov/broadlinkac2mqtt/app/service/models"
)

func DeviceConfigsFromDiscovery(devices []Device, hidden []string, temperatureUnit string) []models.DeviceConfig {
	if temperatureUnit == "" {
		temperatureUnit = models.Celsius
	}

	result := make([]models.DeviceConfig, 0, len(devices))
	for _, device := range devices {
		if IsHidden(device, hidden) {
			continue
		}
		name := device.FriendlyName
		if name == "" {
			name = device.Mac
		}
		result = append(result, models.DeviceConfig{
			Mac:             device.Mac,
			Name:            name,
			TemperatureUnit: temperatureUnit,
			Backend:         models.BackendCloud,
			CloudEndpointID: device.EndpointID,
			CloudProductID:  device.ProductID,
		})
	}
	return result
}

func IsHidden(device Device, hidden []string) bool {
	for _, item := range hidden {
		item = strings.TrimSpace(strings.ToLower(item))
		if item == "" {
			continue
		}
		if item == strings.ToLower(device.EndpointID) ||
			item == strings.ToLower(device.FriendlyName) ||
			item == strings.ToLower(device.Mac) {
			return true
		}
		if normalized, err := models.NormalizeMac(item); err == nil && normalized == device.Mac {
			return true
		}
	}
	return false
}

func MergeDevices(local []models.DeviceConfig, cloud []models.DeviceConfig) []models.DeviceConfig {
	seen := make(map[string]struct{}, len(local)+len(cloud))
	merged := make([]models.DeviceConfig, 0, len(local)+len(cloud))

	for _, device := range local {
		seen[device.Mac] = struct{}{}
		merged = append(merged, device)
	}
	for _, device := range cloud {
		if _, exists := seen[device.Mac]; exists {
			continue
		}
		seen[device.Mac] = struct{}{}
		merged = append(merged, device)
	}
	return merged
}

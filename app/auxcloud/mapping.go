package auxcloud

import (
	"math"

	"github.com/ArtemVladimirov/broadlinkac2mqtt/app/service/models"
)

const (
	cloudModeCool = 0
	cloudModeHeat = 1
	cloudModeDry  = 2
	cloudModeFan  = 3
	cloudModeAuto = 4

	cloudFanAuto   = 0
	cloudFanLow    = 1
	cloudFanMedium = 2
	cloudFanHigh   = 3
	cloudFanTurbo  = 4
	cloudFanMute   = 5
)

func StateFromParams(params map[string]int) models.DeviceState {
	power := firstPresent(params, "pwr", "ac_pwr")
	mode := params["ac_mode"]
	if _, ok := params["ac_mode"]; !ok {
		mode = cloudModeAuto
	}

	temp := firstPresent(params, "temp", "ac_temp")
	target := float32(temp) / 10
	if target == 0 {
		target = 0
	}

	fan := cloudFanAuto
	if value, ok := params["ac_mark"]; ok {
		fan = value
	}

	if params["tempunit"] == 0 {
		if target > 45 {
			target = (target - 32) / 1.8
		}
	}

	status := models.DeviceStatusHass{
		Temperature:   float32(math.Round(float64(target)*10) / 10),
		Mode:          cloudModeToHA(power, mode),
		FanMode:       cloudFanToHA(fan),
		SwingMode:     cloudSwingToHA(params["ac_vdir"], params["ac_hdir"]),
		DisplaySwitch: onOffFromInt(params["scrdisp"]),
		HealthSwitch:  onOffFromInt(params["ac_health"]),
		CleanSwitch:   onOffFromInt(params["ac_clean"]),
		MildewSwitch:  onOffFromInt(params["mldprf"]),
	}

	state := models.DeviceState{Status: status}
	if ambient, ok := params["envtemp"]; ok {
		current := float32(ambient) / 10
		if params["tempunit"] == 0 && current > 45 {
			current = (current - 32) / 1.8
		}
		rounded := float32(math.Round(float64(current)*10) / 10)
		state.AmbientTemp = &rounded
	}

	return state
}

func ParamsFromUpdate(input *models.UpdateDeviceStatesInput) map[string]int {
	params := make(map[string]int)
	if input == nil {
		return params
	}

	if input.Mode != nil {
		if *input.Mode == "off" {
			params["pwr"] = 0
		} else {
			params["pwr"] = 1
			params["ac_mode"] = haModeToCloud(*input.Mode)
		}
	}
	if input.Temperature != nil {
		params["temp"] = int(math.Round(float64(*input.Temperature) * 10))
	}
	if input.FanMode != nil {
		params["ac_mark"] = haFanToCloud(*input.FanMode)
	}
	if input.SwingMode != nil {
		swing := 0
		if *input.SwingMode == "swing" {
			swing = 1
		}
		params["ac_vdir"] = swing
		params["ac_hdir"] = swing
	}
	if input.IsDisplayOn != nil {
		params["scrdisp"] = boolToInt(*input.IsDisplayOn)
	}
	if input.IsHealthOn != nil {
		params["ac_health"] = boolToInt(*input.IsHealthOn)
	}
	if input.IsCleanOn != nil {
		params["ac_clean"] = boolToInt(*input.IsCleanOn)
	}
	if input.IsMildewOn != nil {
		params["mldprf"] = boolToInt(*input.IsMildewOn)
	}
	return params
}

func capabilitiesFromParams(params map[string]int) models.DeviceCapabilities {
	caps := models.DefaultCloudCapabilities()
	caps.DisplaySwitch = hasAnyKey(params, "scrdisp")
	caps.HealthSwitch = hasAnyKey(params, "ac_health")
	caps.CleanSwitch = hasAnyKey(params, "ac_clean")
	caps.MildewSwitch = hasAnyKey(params, "mldprf")
	if !hasAnyKey(params, "ac_vdir", "ac_hdir") {
		caps.SwingModes = []string{"off"}
	}
	return caps
}

func cloudModeToHA(power, mode int) string {
	if power != 1 {
		return "off"
	}
	switch mode {
	case cloudModeCool:
		return "cool"
	case cloudModeHeat:
		return "heat"
	case cloudModeDry:
		return "dry"
	case cloudModeFan:
		return "fan_only"
	default:
		return "auto"
	}
}

func haModeToCloud(mode string) int {
	switch mode {
	case "cool":
		return cloudModeCool
	case "heat":
		return cloudModeHeat
	case "dry":
		return cloudModeDry
	case "fan_only":
		return cloudModeFan
	default:
		return cloudModeAuto
	}
}

func cloudFanToHA(fan int) string {
	switch fan {
	case cloudFanLow:
		return "low"
	case cloudFanMedium:
		return "medium"
	case cloudFanHigh:
		return "high"
	case cloudFanTurbo:
		return "turbo"
	case cloudFanMute:
		return "mute"
	default:
		return "auto"
	}
}

func haFanToCloud(mode string) int {
	switch mode {
	case "low":
		return cloudFanLow
	case "medium":
		return cloudFanMedium
	case "high":
		return cloudFanHigh
	case "turbo":
		return cloudFanTurbo
	case "mute":
		return cloudFanMute
	default:
		return cloudFanAuto
	}
}

func cloudSwingToHA(vertical, horizontal int) string {
	if vertical == 1 || horizontal == 1 {
		return "swing"
	}
	return "off"
}

func firstPresent(params map[string]int, keys ...string) int {
	for _, key := range keys {
		if value, ok := params[key]; ok {
			return value
		}
	}
	return 0
}

func hasAnyKey(params map[string]int, keys ...string) bool {
	for _, key := range keys {
		if _, ok := params[key]; ok {
			return true
		}
	}
	return false
}

func onOffFromInt(value int) string {
	if value == 1 {
		return "ON"
	}
	return "OFF"
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

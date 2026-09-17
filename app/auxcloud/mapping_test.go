package auxcloud

import (
	"testing"

	"github.com/ArtemVladimirov/broadlinkac2mqtt/app/service/models"
)

func TestStateFromParams(t *testing.T) {
	state := StateFromParams(map[string]int{
		"pwr":       1,
		"ac_mode":   cloudModeHeat,
		"temp":      245,
		"envtemp":   261,
		"ac_mark":   cloudFanMute,
		"ac_vdir":   1,
		"ac_hdir":   0,
		"scrdisp":   1,
		"ac_health": 0,
		"ac_clean":  1,
		"mldprf":    0,
	})

	if state.Status.Mode != "heat" {
		t.Fatalf("mode = %q, want heat", state.Status.Mode)
	}
	if state.Status.Temperature != 24.5 {
		t.Fatalf("temp = %v, want 24.5", state.Status.Temperature)
	}
	if state.AmbientTemp == nil || *state.AmbientTemp != 26.1 {
		t.Fatalf("ambient = %v, want 26.1", state.AmbientTemp)
	}
	if state.Status.FanMode != "mute" {
		t.Fatalf("fan = %q, want mute", state.Status.FanMode)
	}
	if state.Status.SwingMode != "swing" {
		t.Fatalf("swing = %q, want swing", state.Status.SwingMode)
	}
	if state.Status.DisplaySwitch != "ON" || state.Status.CleanSwitch != "ON" {
		t.Fatalf("switches display=%s clean=%s", state.Status.DisplaySwitch, state.Status.CleanSwitch)
	}
}

func TestStateFromParamsMissingAmbient(t *testing.T) {
	state := StateFromParams(map[string]int{
		"pwr":     0,
		"ac_mode": cloudModeCool,
		"ac_temp": 180,
	})
	if state.Status.Mode != "off" {
		t.Fatalf("mode = %q, want off", state.Status.Mode)
	}
	if state.AmbientTemp != nil {
		t.Fatal("ambient must be omitted when envtemp is missing")
	}
	if state.Status.Temperature != 18 {
		t.Fatalf("temp = %v, want 18", state.Status.Temperature)
	}
}

func TestParamsFromUpdate(t *testing.T) {
	mode := "cool"
	temp := float32(23.5)
	fan := "turbo"
	swing := "swing"
	display := true

	got := ParamsFromUpdate(&models.UpdateDeviceStatesInput{
		Mode:        &mode,
		Temperature: &temp,
		FanMode:     &fan,
		SwingMode:   &swing,
		IsDisplayOn: &display,
	})

	if got["pwr"] != 1 || got["ac_mode"] != cloudModeCool {
		t.Fatalf("mode params = %#v", got)
	}
	if got["temp"] != 235 {
		t.Fatalf("temp = %d, want 235", got["temp"])
	}
	if got["ac_mark"] != cloudFanTurbo {
		t.Fatalf("fan = %d, want turbo", got["ac_mark"])
	}
	if got["ac_vdir"] != 1 || got["ac_hdir"] != 1 {
		t.Fatalf("swing params = %#v", got)
	}
	if got["scrdisp"] != 1 {
		t.Fatalf("display = %d, want 1", got["scrdisp"])
	}
}

func TestCapabilitiesFromParams(t *testing.T) {
	caps := capabilitiesFromParams(map[string]int{
		"pwr":     1,
		"ac_mode": 0,
		"scrdisp": 1,
		"ac_vdir": 0,
	})
	if !caps.DisplaySwitch || caps.HealthSwitch || caps.CleanSwitch {
		t.Fatalf("unexpected switch caps: %+v", caps)
	}
	if !caps.HasSwingMode("swing") {
		t.Fatal("swing should be available when ac_vdir is present")
	}
}

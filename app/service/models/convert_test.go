package models

import "testing"

func TestConvertToDeviceStatusHASwingMode(t *testing.T) {
	tests := []struct {
		name             string
		fixationVertical byte
		want             string
	}{
		{name: "stop is off", fixationVertical: 0b00000000, want: "off"},
		{name: "top", fixationVertical: 0b00000001, want: "top"},
		{name: "middle1", fixationVertical: 0b00000010, want: "middle1"},
		{name: "middle2", fixationVertical: 0b00000011, want: "middle2"},
		{name: "middle3", fixationVertical: 0b00000100, want: "middle3"},
		{name: "bottom", fixationVertical: 0b00000101, want: "bottom"},
		{name: "swing", fixationVertical: 0b00000110, want: "swing"},
		{name: "auto", fixationVertical: 0b00000111, want: "auto"},
		{name: "unknown falls back to off", fixationVertical: 0b00001000, want: "off"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (DeviceStatusRaw{FixationVertical: tt.fixationVertical}).ConvertToDeviceStatusHA(false).SwingMode
			if got != tt.want {
				t.Fatalf("SwingMode = %q, want %q", got, tt.want)
			}
			if got == "" {
				t.Fatal("SwingMode must never be empty")
			}
		})
	}
}

func TestConvertToDeviceStatusHASwingModeMuteDoesNotClearSwing(t *testing.T) {
	raw := DeviceStatusRaw{
		Mute:             StatusOn,
		FanSpeed:         0,
		FixationVertical: 0b00000110,
		Power:            StatusOn,
		Mode:             0b00000001,
		Temperature:      25,
	}

	got := raw.ConvertToDeviceStatusHA(false)
	if got.FanMode != "mute" {
		t.Fatalf("FanMode = %q, want mute", got.FanMode)
	}
	if got.SwingMode != "swing" {
		t.Fatalf("SwingMode = %q, want swing", got.SwingMode)
	}
}

func TestConvertToDeviceStatusHASwingModeZeroWhileMute(t *testing.T) {
	raw := DeviceStatusRaw{
		Mute:             StatusOn,
		FanSpeed:         0,
		FixationVertical: 0,
		Power:            StatusOn,
		Mode:             0b00000001,
		Temperature:      25,
	}

	got := raw.ConvertToDeviceStatusHA(false)
	if got.FanMode != "mute" {
		t.Fatalf("FanMode = %q, want mute", got.FanMode)
	}
	if got.SwingMode != "off" {
		t.Fatalf("SwingMode = %q, want off (never empty)", got.SwingMode)
	}
}

func TestVerticalFixationStatusesCoverAllThreeBitValues(t *testing.T) {
	for value := 0; value <= 0b00000111; value++ {
		name, ok := VerticalFixationStatuses[value]
		if !ok || name == "" {
			t.Fatalf("missing swing mapping for %d", value)
		}
		if VerticalFixationStatusesInvert[name] != value {
			t.Fatalf("invert mapping for %q is %d, want %d", name, VerticalFixationStatusesInvert[name], value)
		}
	}
}

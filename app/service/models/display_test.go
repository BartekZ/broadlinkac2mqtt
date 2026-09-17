package models

import "testing"

func TestDisplayByteToHass(t *testing.T) {
	tests := []struct {
		name          string
		display       byte
		invertDisplay bool
		want          string
	}{
		{name: "default byte 0 is ON", display: 0, invertDisplay: false, want: "ON"},
		{name: "default byte 1 is OFF", display: 1, invertDisplay: false, want: "OFF"},
		{name: "invert byte 0 is OFF", display: 0, invertDisplay: true, want: "OFF"},
		{name: "invert byte 1 is ON", display: 1, invertDisplay: true, want: "ON"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DisplayByteToHass(tt.display, tt.invertDisplay)
			if got != tt.want {
				t.Fatalf("DisplayByteToHass(%d, %v) = %q, want %q", tt.display, tt.invertDisplay, got, tt.want)
			}
		})
	}
}

func TestHassDisplayToByte(t *testing.T) {
	tests := []struct {
		name          string
		isOn          bool
		invertDisplay bool
		want          byte
	}{
		{name: "default ON is 0", isOn: true, invertDisplay: false, want: 0},
		{name: "default OFF is 1", isOn: false, invertDisplay: false, want: 1},
		{name: "invert ON is 1", isOn: true, invertDisplay: true, want: 1},
		{name: "invert OFF is 0", isOn: false, invertDisplay: true, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HassDisplayToByte(tt.isOn, tt.invertDisplay)
			if got != tt.want {
				t.Fatalf("HassDisplayToByte(%v, %v) = %d, want %d", tt.isOn, tt.invertDisplay, got, tt.want)
			}
		})
	}
}

func TestConvertToDeviceStatusHADisplay(t *testing.T) {
	raw := DeviceStatusRaw{Display: 1}

	if got := raw.ConvertToDeviceStatusHA(false).DisplaySwitch; got != "OFF" {
		t.Fatalf("default invert DisplaySwitch = %q, want OFF", got)
	}
	if got := raw.ConvertToDeviceStatusHA(true).DisplaySwitch; got != "ON" {
		t.Fatalf("invert_display DisplaySwitch = %q, want ON", got)
	}
}

func TestMildewByteToHass(t *testing.T) {
	if got := MildewByteToHass(0); got != "OFF" {
		t.Fatalf("MildewByteToHass(0) = %q, want OFF", got)
	}
	if got := MildewByteToHass(1); got != "ON" {
		t.Fatalf("MildewByteToHass(1) = %q, want ON", got)
	}
}

func TestHassMildewToByte(t *testing.T) {
	if got := HassMildewToByte(true); got != 1 {
		t.Fatalf("HassMildewToByte(true) = %d, want 1", got)
	}
	if got := HassMildewToByte(false); got != 0 {
		t.Fatalf("HassMildewToByte(false) = %d, want 0", got)
	}
}

func TestConvertToDeviceStatusHAMildew(t *testing.T) {
	if got := (DeviceStatusRaw{Mildew: 0}).ConvertToDeviceStatusHA(false).MildewSwitch; got != "OFF" {
		t.Fatalf("MildewSwitch = %q, want OFF", got)
	}
	if got := (DeviceStatusRaw{Mildew: 1}).ConvertToDeviceStatusHA(false).MildewSwitch; got != "ON" {
		t.Fatalf("MildewSwitch = %q, want ON", got)
	}
}

func TestOnOffByteToHassCleanHealth(t *testing.T) {
	if got := (DeviceStatusRaw{Clean: 0, Health: 1}).ConvertToDeviceStatusHA(false); got.CleanSwitch != "OFF" || got.HealthSwitch != "ON" {
		t.Fatalf("CleanSwitch=%q HealthSwitch=%q, want OFF/ON", got.CleanSwitch, got.HealthSwitch)
	}
	if got := HassOnOffToByte(true); got != 1 {
		t.Fatalf("HassOnOffToByte(true) = %d, want 1", got)
	}
	if got := HassOnOffToByte(false); got != 0 {
		t.Fatalf("HassOnOffToByte(false) = %d, want 0", got)
	}
}

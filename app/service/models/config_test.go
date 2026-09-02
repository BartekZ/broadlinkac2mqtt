package models

import "testing"

func TestNormalizeMac(t *testing.T) {
	got, err := NormalizeMac("34:EA:34:5b:0f:d4")
	if err != nil {
		t.Fatalf("NormalizeMac returned error: %v", err)
	}
	if got != "34ea345b0fd4" {
		t.Fatalf("NormalizeMac = %q, want 34ea345b0fd4", got)
	}

	if _, err = NormalizeMac("short"); err == nil {
		t.Fatal("expected invalid mac error")
	}
}

func TestDeviceConfigValidateLocalAndCloud(t *testing.T) {
	local := DeviceConfig{
		Mac:             "34ea345b0fd4",
		Ip:              "192.168.1.12",
		Name:            "AC",
		Port:            80,
		TemperatureUnit: Celsius,
	}
	if err := local.Validate(); err != nil {
		t.Fatalf("local validate: %v", err)
	}
	if local.Backend != BackendLocal {
		t.Fatalf("backend = %q, want local", local.Backend)
	}

	cloud := DeviceConfig{
		Mac:             "34:ea:345b0fd5",
		Name:            "Cloud AC",
		TemperatureUnit: Celsius,
		Backend:         BackendCloud,
		CloudEndpointID: "endpoint-1",
	}
	if err := cloud.Validate(); err != nil {
		t.Fatalf("cloud validate: %v", err)
	}
	if cloud.Mac != "34ea345b0fd5" {
		t.Fatalf("cloud mac = %q", cloud.Mac)
	}

	invalid := DeviceConfig{
		Mac:             "34ea345b0fd4",
		TemperatureUnit: Celsius,
		Backend:         BackendCloud,
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected cloud device id error")
	}
}

func TestDefaultCapabilities(t *testing.T) {
	local := DefaultLocalCapabilities()
	if !local.HasSwingMode("middle1") || !local.DisplaySwitch {
		t.Fatal("local capabilities missing expected values")
	}

	cloud := DefaultCloudCapabilities()
	if cloud.HasSwingMode("middle1") {
		t.Fatal("cloud capabilities should not include positional swing")
	}
	if !cloud.HasSwingMode("off") || !cloud.HasSwingMode("swing") {
		t.Fatal("cloud capabilities should include off/swing")
	}
}

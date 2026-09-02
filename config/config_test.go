package config

import "testing"

func TestCloudValidate(t *testing.T) {
	disabled := Cloud{}
	if err := disabled.Validate(); err != nil {
		t.Fatalf("disabled cloud should be valid: %v", err)
	}

	enabled := Cloud{Enabled: true, Email: "a@b.c", Password: "secret", Region: "eu"}
	if err := enabled.Validate(); err != nil {
		t.Fatalf("valid cloud: %v", err)
	}

	if err := (Cloud{Enabled: true, Email: "a@b.c"}).Validate(); err == nil {
		t.Fatal("expected missing password error")
	}
	if err := (Cloud{Enabled: true, Email: "a@b.c", Password: "x", Region: "ru"}).Validate(); err == nil {
		t.Fatal("expected invalid region error")
	}
}

func TestCloudAutoDiscoverDefault(t *testing.T) {
	if !(Cloud{Enabled: true}).AutoDiscoverEnabled() {
		t.Fatal("auto discover should default to true")
	}
	disabled := false
	if (Cloud{AutoDiscover: &disabled}).AutoDiscoverEnabled() {
		t.Fatal("explicit false should disable auto discover")
	}
}

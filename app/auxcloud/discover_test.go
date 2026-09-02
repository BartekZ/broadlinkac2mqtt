package auxcloud

import (
	"testing"

	"github.com/ArtemVladimirov/broadlinkac2mqtt/app/service/models"
)

func TestDeviceConfigsFromDiscoveryAndHidden(t *testing.T) {
	devices := []Device{
		{EndpointID: "ep-1", FriendlyName: "Bedroom AC", ProductID: "pid", Mac: "34ea345b0fd4"},
		{EndpointID: "ep-2", FriendlyName: "Kitchen AC", ProductID: "pid", Mac: "34ea345b0fd5"},
	}

	got := DeviceConfigsFromDiscovery(devices, []string{"Bedroom AC"}, "C")
	if len(got) != 1 || got[0].Mac != "34ea345b0fd5" {
		t.Fatalf("got %+v", got)
	}
	if got[0].Backend != models.BackendCloud || got[0].CloudEndpointID != "ep-2" {
		t.Fatalf("cloud config = %+v", got[0])
	}
}

func TestMergeDevicesSkipsMacCollisions(t *testing.T) {
	local := []models.DeviceConfig{{
		Mac: "34ea345b0fd4", Name: "Local", Backend: models.BackendLocal, Ip: "127.0.0.1", Port: 80,
	}}
	cloud := []models.DeviceConfig{{
		Mac: "34ea345b0fd4", Name: "Cloud duplicate", Backend: models.BackendCloud, CloudEndpointID: "ep-1",
	}, {
		Mac: "34ea345b0fd5", Name: "Cloud only", Backend: models.BackendCloud, CloudEndpointID: "ep-2",
	}}

	got := MergeDevices(local, cloud)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Name != "Local" || got[1].Name != "Cloud only" {
		t.Fatalf("merged = %+v", got)
	}
}

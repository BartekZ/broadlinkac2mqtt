package main

import (
	"testing"

	workspaceServiceModels "github.com/ArtemVladimirov/broadlinkac2mqtt/app/service/models"
	"github.com/ArtemVladimirov/broadlinkac2mqtt/config"
)

func TestLocalDeviceConfigsSkipsEmptyAndValidates(t *testing.T) {
	got, err := localDeviceConfigs([]config.Devices{
		{},
		{Ip: "192.168.1.12", Mac: "34EA345B0FD4", Name: "Childroom", Port: 80, TemperatureUnit: "C"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Mac != "34ea345b0fd4" || got[0].Backend != workspaceServiceModels.BackendLocal {
		t.Fatalf("got %+v", got[0])
	}
}

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	modelsMqtt "github.com/ArtemVladimirov/broadlinkac2mqtt/app/mqtt/models"
	"github.com/ArtemVladimirov/broadlinkac2mqtt/app/repository/cache"
	modelsRepo "github.com/ArtemVladimirov/broadlinkac2mqtt/app/repository/models"
	"github.com/ArtemVladimirov/broadlinkac2mqtt/app/service/models"
)

type recordingPublisher struct {
	modes      []string
	swingModes []string
	fanModes   []string
	temps      []float32
	ambient    []float32
	climates   []modelsMqtt.ClimateDiscoveryTopic
	switches   []modelsMqtt.SwitchDiscoveryTopic
}

func (p *recordingPublisher) PublishClimateDiscoveryTopic(_ context.Context, input modelsMqtt.PublishClimateDiscoveryTopicInput) error {
	p.climates = append(p.climates, input.Topic)
	return nil
}
func (p *recordingPublisher) PublishSwitchDiscoveryTopic(_ context.Context, input modelsMqtt.PublishSwitchDiscoveryTopicInput) error {
	p.switches = append(p.switches, input.Topic)
	return nil
}
func (p *recordingPublisher) PublishAmbientTemp(_ context.Context, input *modelsMqtt.PublishAmbientTempInput) error {
	p.ambient = append(p.ambient, input.Temperature)
	return nil
}
func (p *recordingPublisher) PublishTemperature(_ context.Context, input *modelsMqtt.PublishTemperatureInput) error {
	p.temps = append(p.temps, input.Temperature)
	return nil
}
func (p *recordingPublisher) PublishMode(_ context.Context, input *modelsMqtt.PublishModeInput) error {
	p.modes = append(p.modes, input.Mode)
	return nil
}
func (p *recordingPublisher) PublishSwingMode(_ context.Context, input *modelsMqtt.PublishSwingModeInput) error {
	p.swingModes = append(p.swingModes, input.SwingMode)
	return nil
}
func (p *recordingPublisher) PublishFanMode(_ context.Context, input *modelsMqtt.PublishFanModeInput) error {
	p.fanModes = append(p.fanModes, input.FanMode)
	return nil
}
func (p *recordingPublisher) PublishAvailability(context.Context, *modelsMqtt.PublishAvailabilityInput) error {
	return nil
}
func (p *recordingPublisher) PublishDisplaySwitch(context.Context, *modelsMqtt.PublishDisplaySwitchInput) error {
	return nil
}
func (p *recordingPublisher) PublishMildewSwitch(context.Context, *modelsMqtt.PublishMildewSwitchInput) error {
	return nil
}
func (p *recordingPublisher) PublishCleanSwitch(context.Context, *modelsMqtt.PublishCleanSwitchInput) error {
	return nil
}
func (p *recordingPublisher) PublishHealthSwitch(context.Context, *modelsMqtt.PublishHealthSwitchInput) error {
	return nil
}

type fakeBackend struct {
	state   models.DeviceState
	caps    models.DeviceCapabilities
	applied *models.UpdateDeviceStatesInput
	minGap  time.Duration
}

func (f *fakeBackend) Authenticate(context.Context, string) error {
	return nil
}
func (f *fakeBackend) ReadState(context.Context, string) (*models.DeviceState, error) {
	cp := f.state
	return &cp, nil
}
func (f *fakeBackend) ReadAmbient(context.Context, string) (*float32, error) {
	return f.state.AmbientTemp, nil
}
func (f *fakeBackend) ApplyState(_ context.Context, _ string, input *models.UpdateDeviceStatesInput) error {
	f.applied = input
	return nil
}
func (f *fakeBackend) Capabilities(context.Context, string) models.DeviceCapabilities {
	if len(f.caps.Modes) == 0 {
		return models.DefaultCloudCapabilities()
	}
	return f.caps
}
func (f *fakeBackend) MinRequestGap() time.Duration {
	if f.minGap == 0 {
		return time.Second
	}
	return f.minGap
}

func TestGetDeviceStatesPublishesCloudStateWithoutRaw(t *testing.T) {
	publisher := &recordingPublisher{}
	backend := &fakeBackend{
		state: models.DeviceState{
			Status: models.DeviceStatusHass{
				Mode:          "cool",
				FanMode:       "auto",
				SwingMode:     "off",
				Temperature:   24,
				DisplaySwitch: "ON",
				MildewSwitch:  "OFF",
				CleanSwitch:   "OFF",
				HealthSwitch:  "OFF",
			},
		},
		caps: models.DefaultCloudCapabilities(),
	}
	store := cache.NewCache()
	svc := NewService("aircon", 10, publisher, nil, store, backend).(*service)

	mac := "34ea345b0fd4"
	err := svc.CreateDevice(context.Background(), &models.CreateDeviceInput{
		Config: models.DeviceConfig{
			Mac:             mac,
			Name:            "Cloud AC",
			TemperatureUnit: models.Celsius,
			Backend:         models.BackendCloud,
			CloudEndpointID: "ep-1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if err = svc.GetDeviceStates(context.Background(), &models.GetDeviceStatesInput{Mac: mac}); err != nil {
		t.Fatal(err)
	}
	if len(publisher.modes) != 1 || publisher.modes[0] != "cool" {
		t.Fatalf("published modes = %v", publisher.modes)
	}
	_, rawErr := store.ReadDeviceStatusRaw(context.Background(), &modelsRepo.ReadDeviceStatusRawInput{Mac: mac})
	if !errors.Is(rawErr, modelsRepo.ErrorDeviceStatusRawNotFound) {
		t.Fatalf("cloud backend must not persist local raw status, err=%v", rawErr)
	}
}

func TestPublishDiscoveryUsesCloudCapabilities(t *testing.T) {
	publisher := &recordingPublisher{}
	backend := &fakeBackend{
		caps: models.DeviceCapabilities{
			Modes:         []string{"auto", "off", "cool", "heat"},
			FanModes:      []string{"auto", "low", "medium", "high"},
			SwingModes:    []string{"off", "swing"},
			DisplaySwitch: true,
		},
	}
	store := cache.NewCache()
	svc := NewService("aircon", 10, publisher, nil, store, backend).(*service)

	mac := "34ea345b0fd4"
	cfg := models.DeviceConfig{
		Mac:             mac,
		Name:            "Cloud AC",
		TemperatureUnit: models.Celsius,
		Backend:         models.BackendCloud,
		CloudEndpointID: "ep-1",
	}
	if err := svc.CreateDevice(context.Background(), &models.CreateDeviceInput{Config: cfg}); err != nil {
		t.Fatal(err)
	}

	if err := svc.PublishDiscoveryTopic(context.Background(), &models.PublishDiscoveryTopicInput{Device: cfg}); err != nil {
		t.Fatal(err)
	}
	if len(publisher.climates) != 1 {
		t.Fatalf("climates = %d", len(publisher.climates))
	}
	if len(publisher.climates[0].SwingModes) != 2 {
		t.Fatalf("swing modes = %v", publisher.climates[0].SwingModes)
	}
	if len(publisher.switches) != 1 || publisher.switches[0].Name != "Screen" {
		t.Fatalf("switches = %+v", publisher.switches)
	}
}

func TestUpdateDeviceStatesDelegatesToCloudBackend(t *testing.T) {
	backend := &fakeBackend{caps: models.DefaultCloudCapabilities()}
	store := cache.NewCache()
	svc := NewService("aircon", 10, &recordingPublisher{}, nil, store, backend).(*service)
	mac := "34ea345b0fd4"
	if err := svc.CreateDevice(context.Background(), &models.CreateDeviceInput{
		Config: models.DeviceConfig{Mac: mac, Name: "AC", TemperatureUnit: models.Celsius, Backend: models.BackendCloud, CloudEndpointID: "ep-1"},
	}); err != nil {
		t.Fatal(err)
	}

	mode := "off"
	if err := svc.UpdateDeviceStates(context.Background(), &models.UpdateDeviceStatesInput{Mac: mac, Mode: &mode}); err != nil {
		t.Fatal(err)
	}
	if backend.applied == nil || backend.applied.Mode == nil || *backend.applied.Mode != "off" {
		t.Fatalf("applied = %+v", backend.applied)
	}
}

func TestHasKnownStateUsesHassCache(t *testing.T) {
	store := cache.NewCache()
	svc := NewService("aircon", 10, &recordingPublisher{}, nil, store, &fakeBackend{}).(*service)
	mac := "34ea345b0fd4"
	if err := svc.CreateDevice(context.Background(), &models.CreateDeviceInput{
		Config: models.DeviceConfig{Mac: mac, Name: "AC", TemperatureUnit: models.Celsius, Backend: models.BackendCloud, CloudEndpointID: "ep-1"},
	}); err != nil {
		t.Fatal(err)
	}

	m := &deviceMonitor{s: svc, mac: mac}
	if m.hasKnownState(context.Background()) {
		t.Fatal("expected no known state")
	}
}

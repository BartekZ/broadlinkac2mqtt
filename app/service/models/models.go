package models

import (
	"errors"
	"strings"
	"time"
	"unicode"
)

const (
	BackendLocal = "local"
	BackendCloud = "cloud"
)

type Device struct {
	Config DeviceConfig
	Auth   DeviceAuth
}

type DeviceConfig struct {
	Mac             string
	Ip              string
	Name            string
	Port            uint16
	TemperatureUnit string
	InvertDisplay   bool
	Backend         string
	CloudEndpointID string
	CloudProductID  string
}

func (input DeviceConfig) IsCloud() bool {
	return strings.EqualFold(input.Backend, BackendCloud)
}

func (input *DeviceConfig) Validate() error {
	if input.Backend == "" {
		input.Backend = BackendLocal
	}
	if !strings.EqualFold(input.Backend, BackendLocal) && !strings.EqualFold(input.Backend, BackendCloud) {
		return errors.New("unknown device backend")
	}

	mac, err := NormalizeMac(input.Mac)
	if err != nil {
		if input.IsCloud() {
			return errors.New("cloud device mac is missing or invalid")
		}
		return errors.New("mac address is wrong")
	}
	input.Mac = mac

	if input.TemperatureUnit != Celsius && input.TemperatureUnit != Fahrenheit {
		return errors.New("unknown temperature unit")
	}

	if input.IsCloud() {
		if input.CloudEndpointID == "" {
			return errors.New("cloud device id is required")
		}
		return nil
	}

	if input.Ip == "" {
		return errors.New("ip is required for local devices")
	}
	if input.Port == 0 {
		return errors.New("port is required for local devices")
	}

	return nil
}

func NormalizeMac(mac string) (string, error) {
	var b strings.Builder
	b.Grow(12)
	for _, r := range strings.ToLower(mac) {
		if unicode.Is(unicode.ASCII_Hex_Digit, r) {
			b.WriteRune(r)
		}
	}
	if b.Len() != 12 {
		return "", errors.New("mac address is wrong")
	}
	return b.String(), nil
}

type DeviceAuth struct {
	LastMessageId int
	DevType       int
	Id            [4]byte
	Key           []byte
	Iv            []byte
}

type DeviceStatusHass struct {
	FanMode       string
	SwingMode     string
	Mode          string
	Temperature   float32
	DisplaySwitch string
	MildewSwitch  string
	CleanSwitch   string
	HealthSwitch  string
}

type DeviceState struct {
	Status      DeviceStatusHass
	AmbientTemp *float32
	Raw         *DeviceStatusRaw
}

type DeviceCapabilities struct {
	Modes         []string
	FanModes      []string
	SwingModes    []string
	DisplaySwitch bool
	MildewSwitch  bool
	CleanSwitch   bool
	HealthSwitch  bool
}

func DefaultLocalCapabilities() DeviceCapabilities {
	return DeviceCapabilities{
		Modes:         []string{"auto", "off", "cool", "heat", "dry", "fan_only"},
		FanModes:      []string{"auto", "low", "medium", "high", "turbo", "mute"},
		SwingModes:    []string{"off", "top", "middle1", "middle2", "middle3", "bottom", "swing", "auto"},
		DisplaySwitch: true,
		MildewSwitch:  true,
		CleanSwitch:   true,
		HealthSwitch:  true,
	}
}

func DefaultCloudCapabilities() DeviceCapabilities {
	return DeviceCapabilities{
		Modes:         []string{"auto", "off", "cool", "heat", "dry", "fan_only"},
		FanModes:      []string{"auto", "low", "medium", "high", "turbo", "mute"},
		SwingModes:    []string{"off", "swing"},
		DisplaySwitch: true,
		MildewSwitch:  true,
		CleanSwitch:   true,
		HealthSwitch:  true,
	}
}

func (c DeviceCapabilities) HasValue(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func (c DeviceCapabilities) HasMode(mode string) bool {
	return c.HasValue(c.Modes, mode)
}

func (c DeviceCapabilities) HasFanMode(mode string) bool {
	return c.HasValue(c.FanModes, mode)
}

func (c DeviceCapabilities) HasSwingMode(mode string) bool {
	return c.HasValue(c.SwingModes, mode)
}

type DeviceStatusRaw struct {
	UpdatedAt          time.Time
	Temperature        float32
	Power              byte
	FixationVertical   byte
	Mode               byte
	Sleep              byte
	Display            byte
	Mildew             byte
	Health             byte
	FixationHorizontal byte
	FanSpeed           byte
	IFeel              byte
	Mute               byte
	Turbo              byte
	Clean              byte
}

// DisplayByteToHass converts a protocol display byte to Home Assistant ON/OFF.
// Default (invertDisplay=false): byte 0 = ON, byte 1 = OFF.
// invertDisplay=true: byte 1 = ON, byte 0 = OFF.
func DisplayByteToHass(display byte, invertDisplay bool) string {
	on := display == 0
	if invertDisplay {
		on = display == 1
	}
	if on {
		return "ON"
	}
	return "OFF"
}

// HassDisplayToByte converts Home Assistant display state to a protocol byte.
// Default (invertDisplay=false): ON = 0, OFF = 1.
// invertDisplay=true: ON = 1, OFF = 0.
func HassDisplayToByte(isOn bool, invertDisplay bool) byte {
	if invertDisplay == isOn {
		return 1
	}
	return 0
}

func (raw DeviceStatusRaw) ConvertToDeviceStatusHA(invertDisplay bool) (mqttStatus DeviceStatusHass) {
	var deviceStatusMqtt DeviceStatusHass

	// Temperature
	deviceStatusMqtt.Temperature = raw.Temperature

	// Modes
	if raw.Power == StatusOff {
		deviceStatusMqtt.Mode = "off"
	} else {
		status, ok := ModeStatuses[int(raw.Mode)]
		if ok {
			deviceStatusMqtt.Mode = status
		} else {
			deviceStatusMqtt.Mode = "error"
		}
	}

	// Fan Status
	fanStatus, ok := FanStatuses[int(raw.FanSpeed)]
	if ok {
		deviceStatusMqtt.FanMode = fanStatus
	} else {
		deviceStatusMqtt.FanMode = "error"
	}

	if raw.Mute == StatusOn {
		deviceStatusMqtt.FanMode = "mute"
	}

	if raw.Turbo == StatusOn {
		deviceStatusMqtt.FanMode = "turbo"
	}

	// Swing Modes. 0 is STOP in the AUX protocol; some units also report 0
	// after an unsupported swing request (for example swing while mute).
	verticalFixationStatus, ok := VerticalFixationStatuses[int(raw.FixationVertical)]
	if ok {
		deviceStatusMqtt.SwingMode = verticalFixationStatus
	} else {
		deviceStatusMqtt.SwingMode = "off"
	}

	deviceStatusMqtt.DisplaySwitch = DisplayByteToHass(raw.Display, invertDisplay)
	deviceStatusMqtt.MildewSwitch = OnOffByteToHass(raw.Mildew)
	deviceStatusMqtt.CleanSwitch = OnOffByteToHass(raw.Clean)
	deviceStatusMqtt.HealthSwitch = OnOffByteToHass(raw.Health)

	return deviceStatusMqtt
}

func OnOffByteToHass(value byte) string {
	if value == StatusOn {
		return "ON"
	}
	return "OFF"
}

func HassOnOffToByte(isOn bool) byte {
	if isOn {
		return StatusOn
	}
	return StatusOff
}

func MildewByteToHass(mildew byte) string {
	return OnOffByteToHass(mildew)
}

func HassMildewToByte(isOn bool) byte {
	return HassOnOffToByte(isOn)
}

type CreateDeviceInput struct {
	Config DeviceConfig
}

type CreateDeviceReturn struct {
	Device Device
}

type AuthDeviceInput struct {
	Mac string
}

type SendCommandInput struct {
	Command byte
	Payload []byte
	Mac     string
}

type SendCommandReturn struct {
	Payload []byte
}

type GetDeviceAmbientTemperatureInput struct {
	Mac string
}

type GetDeviceStatesInput struct {
	Mac string
}

type PublishDiscoveryTopicInput struct {
	Device DeviceConfig
}

type UpdateFanModeInput struct {
	Mac     string
	FanMode string
}

func (input *UpdateFanModeInput) Validate() error {
	var fanModes = []string{"auto", "low", "medium", "high", "turbo", "mute"}

	for _, fanMode := range fanModes {
		if fanMode == input.FanMode {
			return nil
		}
	}

	return ErrorInvalidParameterFanMode
}

type UpdateModeInput struct {
	Mac  string
	Mode string
}

func (input UpdateModeInput) Validate() error {
	var modes = []string{"auto", "off", "cool", "heat", "dry", "fan_only"}

	for _, mode := range modes {
		if mode == input.Mode {
			return nil
		}
	}

	return ErrorInvalidParameterMode
}

type UpdateSwingModeInput struct {
	Mac       string
	SwingMode string
}

func (input *UpdateSwingModeInput) Validate() error {
	_, ok := VerticalFixationStatusesInvert[input.SwingMode]
	if !ok {
		return ErrorInvalidParameterSwingMode
	}

	return nil

}

type UpdateTemperatureInput struct {
	Mac         string
	Temperature float32
}

func (input UpdateTemperatureInput) Validate() error {
	if input.Temperature > 32 || input.Temperature < 16 {
		return ErrorInvalidParameterTemperature
	}
	return nil
}

type UpdateDeviceStatesInput struct {
	Mac         string
	FanMode     *string
	SwingMode   *string
	Mode        *string
	Temperature *float32
	IsDisplayOn *bool
	IsMildewOn  *bool
	IsCleanOn   *bool
	IsHealthOn  *bool
}

type CreateCommandPayloadReturn struct {
	Payload []byte
}

type UpdateDeviceAvailabilityInput struct {
	Mac          string
	Availability string
}

type StartDeviceMonitoringInput struct {
	Mac string
}

type PublishStatesOnHomeAssistantRestartInput struct {
	Status string
}

type UpdateDisplaySwitchInput struct {
	Mac    string
	Status string
}

func (input *UpdateDisplaySwitchInput) Validate() error {
	if input.Status != "ON" && input.Status != "OFF" {
		return ErrorInvalidParameterDisplayStatus
	}
	return nil
}

type UpdateMildewSwitchInput struct {
	Mac    string
	Status string
}

func (input *UpdateMildewSwitchInput) Validate() error {
	if input.Status != "ON" && input.Status != "OFF" {
		return ErrorInvalidParameterMildewStatus
	}
	return nil
}

type UpdateCleanSwitchInput struct {
	Mac    string
	Status string
}

func (input *UpdateCleanSwitchInput) Validate() error {
	if input.Status != "ON" && input.Status != "OFF" {
		return ErrorInvalidParameterCleanStatus
	}
	return nil
}

type UpdateHealthSwitchInput struct {
	Mac    string
	Status string
}

func (input *UpdateHealthSwitchInput) Validate() error {
	if input.Status != "ON" && input.Status != "OFF" {
		return ErrorInvalidParameterHealthStatus
	}
	return nil
}

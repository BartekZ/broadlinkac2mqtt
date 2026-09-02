package models

import (
	"time"
)

type Device struct {
	Config           DeviceConfig
	Auth             *DeviceAuth
	DeviceStatus     DeviceStatus
	DeviceStatusRaw  *DeviceStatusRaw
	DeviceStatusHass *DeviceStatusHass
	MqttLastMessage  MqttStatus
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

func (c DeviceConfig) IsCloud() bool {
	return c.Backend == "cloud"
}

type DeviceAuth struct {
	LastMessageId int
	DevType       int
	Id            [4]byte
	Key           []byte
	Iv            []byte
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

type DeviceStatus struct {
	Availability *string
	AmbientTemp  *float32
}

type MqttStatus struct {
	FanMode       *MqttFanModeMessage
	SwingMode     *MqttSwingModeMessage
	Mode          *MqttModeMessage
	Temperature   *MqttTemperatureMessage
	DisplaySwitch *MqttDisplaySwitchMessage
	MildewSwitch  *MqttMildewSwitchMessage
	CleanSwitch   *MqttCleanSwitchMessage
	HealthSwitch  *MqttHealthSwitchMessage
}

type ReadDeviceConfigInput struct {
	Mac string
}

type ReadDeviceConfigReturn struct {
	Config DeviceConfig
}

type UpsertDeviceConfigInput struct {
	Config DeviceConfig
}

type ReadDeviceAuthInput struct {
	Mac string
}

type ReadDeviceAuthReturn struct {
	Auth DeviceAuth
}

type UpsertDeviceAuthInput struct {
	Mac  string
	Auth DeviceAuth
}

type UpsertAmbientTempInput struct {
	Mac         string
	Temperature float32
}

type ReadAmbientTempInput struct {
	Mac string
}

type ReadAmbientTempReturn struct {
	Temperature float32
}

type ReadDeviceStatusRawInput struct {
	Mac string
}

type ReadDeviceStatusRawReturn struct {
	Status DeviceStatusRaw
}

type UpsertDeviceStatusRawInput struct {
	Mac    string
	Status DeviceStatusRaw
}

type ReadDeviceStatusHassInput struct {
	Mac string
}

type ReadDeviceStatusHassReturn struct {
	Status DeviceStatusHass
}

type UpsertDeviceStatusHassInput struct {
	Mac    string
	Status DeviceStatusHass
}

type MqttModeMessage struct {
	UpdatedAt time.Time
	Mode      string
}

type UpsertMqttModeMessageInput struct {
	Mac  string
	Mode MqttModeMessage
}

type MqttFanModeMessage struct {
	UpdatedAt time.Time
	FanMode   string
}

type UpsertMqttFanModeMessageInput struct {
	Mac     string
	FanMode MqttFanModeMessage
}

type MqttDisplaySwitchMessage struct {
	UpdatedAt   time.Time
	IsDisplayOn bool
}

type UpsertMqttDisplaySwitchMessageInput struct {
	Mac           string
	DisplaySwitch MqttDisplaySwitchMessage
}

type MqttMildewSwitchMessage struct {
	UpdatedAt  time.Time
	IsMildewOn bool
}

type UpsertMqttMildewSwitchMessageInput struct {
	Mac          string
	MildewSwitch MqttMildewSwitchMessage
}

type MqttCleanSwitchMessage struct {
	UpdatedAt time.Time
	IsCleanOn bool
}

type UpsertMqttCleanSwitchMessageInput struct {
	Mac         string
	CleanSwitch MqttCleanSwitchMessage
}

type MqttHealthSwitchMessage struct {
	UpdatedAt  time.Time
	IsHealthOn bool
}

type UpsertMqttHealthSwitchMessageInput struct {
	Mac          string
	HealthSwitch MqttHealthSwitchMessage
}

type MqttSwingModeMessage struct {
	UpdatedAt time.Time
	SwingMode string
}

type UpsertMqttSwingModeMessageInput struct {
	Mac       string
	SwingMode MqttSwingModeMessage
}

type MqttTemperatureMessage struct {
	UpdatedAt   time.Time
	Temperature float32
}

type UpsertMqttTemperatureMessageInput struct {
	Mac         string
	Temperature MqttTemperatureMessage
}

type ReadMqttMessageInput struct {
	Mac string
}

type ReadMqttMessageReturn struct {
	Temperature *MqttTemperatureMessage
	SwingMode   *MqttSwingModeMessage
	FanMode     *MqttFanModeMessage
	Mode        *MqttModeMessage
	IsDisplayOn *MqttDisplaySwitchMessage
	IsMildewOn  *MqttMildewSwitchMessage
	IsCleanOn   *MqttCleanSwitchMessage
	IsHealthOn  *MqttHealthSwitchMessage
}

type UpsertDeviceAvailabilityInput struct {
	Mac          string
	Availability string
}

type ReadDeviceAvailabilityInput struct {
	Mac string
}

type ReadDeviceAvailabilityReturn struct {
	Availability string
}

type ReadAuthedDevicesReturn struct {
	Macs []string
}

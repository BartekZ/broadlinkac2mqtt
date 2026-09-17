package service

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"strconv"
	"time"

	"github.com/ArtemVladimirov/broadlinkac2mqtt/app"
	modelsRepo "github.com/ArtemVladimirov/broadlinkac2mqtt/app/repository/models"
	"github.com/ArtemVladimirov/broadlinkac2mqtt/app/service/models"
	modelsWeb "github.com/ArtemVladimirov/broadlinkac2mqtt/app/webClient/models"
	"github.com/ArtemVladimirov/broadlinkac2mqtt/pkg/coder"
)

type localBackend struct {
	webClient app.WebClient
	cache     app.Cache
}

func NewLocalBackend(webClient app.WebClient, cache app.Cache) app.DeviceBackend {
	return &localBackend{
		webClient: webClient,
		cache:     cache,
	}
}

func (b *localBackend) MinRequestGap() time.Duration {
	return minUdpGap
}

func (b *localBackend) Capabilities(_ context.Context, _ string) models.DeviceCapabilities {
	return models.DefaultLocalCapabilities()
}

func (b *localBackend) Create(ctx context.Context, mac string) error {
	key := []byte{0x09, 0x76, 0x28, 0x34, 0x3f, 0xe9, 0x9e, 0x23, 0x76, 0x5c, 0x15, 0x13, 0xac, 0xcf, 0x8b, 0x02}
	iv := []byte{0x56, 0x2e, 0x17, 0x99, 0x6d, 0x09, 0x3d, 0x28, 0xdd, 0xb3, 0xba, 0x69, 0x5a, 0x2e, 0x6f, 0x58}

	auth := modelsRepo.DeviceAuth{
		LastMessageId: rand.Intn(0xffff), //nolint:gosec // protocol sequence number, not a secret
		DevType:       0x4E2a,
		Id:            [4]byte{0, 0, 0, 0},
		Key:           key,
		Iv:            iv,
	}

	return b.cache.UpsertDeviceAuth(ctx, &modelsRepo.UpsertDeviceAuthInput{
		Mac:  mac,
		Auth: auth,
	})
}

func (b *localBackend) Authenticate(ctx context.Context, mac string) error {
	payload := [0x50]byte{}
	payload[0x04] = 0x31
	payload[0x05] = 0x31
	payload[0x06] = 0x31
	payload[0x07] = 0x31
	payload[0x08] = 0x31
	payload[0x09] = 0x31
	payload[0x0a] = 0x31
	payload[0x0b] = 0x31
	payload[0x0c] = 0x31
	payload[0x0d] = 0x31
	payload[0x0e] = 0x31
	payload[0x0f] = 0x31
	payload[0x10] = 0x31
	payload[0x11] = 0x31
	payload[0x12] = 0x31
	payload[0x1e] = 0x01
	payload[0x2d] = 0x01
	payload[0x30] = byte('T')
	payload[0x31] = byte('e')
	payload[0x32] = byte('s')
	payload[0x33] = byte('t')
	payload[0x34] = byte(' ')
	payload[0x35] = byte(' ')
	payload[0x36] = byte('1')

	response, err := b.sendCommand(ctx, &models.SendCommandInput{
		Command: 0x65,
		Payload: payload[:],
		Mac:     mac,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to send command", slog.Any("err", err), slog.String("device", mac))
		return err
	}

	if len(response.Payload) >= 0x38 {
		response.Payload = response.Payload[0x38:]
	} else {
		const msg = "response is too short"
		slog.ErrorContext(ctx, msg, slog.String("device", mac), slog.Any("payload", response.Payload))
		return errors.New(msg)
	}

	readDeviceAuthReturn, err := b.cache.ReadDeviceAuth(ctx, &modelsRepo.ReadDeviceAuthInput{Mac: mac})
	if err != nil {
		slog.ErrorContext(ctx, "device not found", slog.Any("err", err), slog.String("device", mac))
		return err
	}
	auth := readDeviceAuthReturn.Auth

	response.Payload, err = coder.Decrypt(auth.Key, auth.Iv, response.Payload)
	if err != nil {
		slog.ErrorContext(ctx, "failed to decode response", slog.Any("err", err), slog.String("device", mac))
		return err
	}

	auth = modelsRepo.DeviceAuth{
		LastMessageId: auth.LastMessageId,
		DevType:       auth.DevType,
		Id:            [4]byte{response.Payload[0], response.Payload[1], response.Payload[2], response.Payload[3]},
		Key:           response.Payload[0x04:0x14],
		Iv:            auth.Iv,
	}

	return b.cache.UpsertDeviceAuth(ctx, &modelsRepo.UpsertDeviceAuthInput{
		Mac:  mac,
		Auth: auth,
	})
}

func (b *localBackend) ReadAmbient(ctx context.Context, mac string) (*float32, error) {
	response, err := b.sendCommand(ctx, &models.SendCommandInput{
		Command: 0x6a,
		Payload: []byte{12, 0, 187, 0, 6, 128, 0, 0, 2, 0, 33, 1, 27, 126, 0, 0},
		Mac:     mac,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to send a command", slog.Any("err", err), slog.String("device", mac))
		return nil, err
	}

	if uint16(response.Payload[0x22])|(uint16(response.Payload[0x23])<<8) != 0 {
		slog.ErrorContext(ctx, "Checksum is incorrect", slog.String("device", mac))
		return nil, models.ErrorInvalidResultPacket
	}

	if len(response.Payload) >= 0x38 {
		response.Payload = response.Payload[0x38:]
	} else {
		slog.ErrorContext(ctx, "response is too short", slog.String("device", mac))
		return nil, models.ErrorInvalidResultPacketLength
	}

	readDeviceAuthReturn, err := b.cache.ReadDeviceAuth(ctx, &modelsRepo.ReadDeviceAuthInput{Mac: mac})
	if err != nil {
		slog.ErrorContext(ctx, "device not found", slog.Any("err", err), slog.String("device", mac))
		return nil, err
	}

	response.Payload, err = coder.Decrypt(readDeviceAuthReturn.Auth.Key, readDeviceAuthReturn.Auth.Iv, response.Payload)
	if err != nil {
		slog.ErrorContext(ctx, "failed to decrypt response", slog.Any("err", err), slog.String("device", mac))
		return nil, err
	}

	response.Payload = response.Payload[2:]
	if len(response.Payload) < 40 {
		return nil, models.ErrorInvalidResultPacketLength
	}

	ambientTemp := float32(response.Payload[15]-0b00100000) + (float32(response.Payload[31]) / 10)
	return &ambientTemp, nil
}

func (b *localBackend) ReadState(ctx context.Context, mac string) (*models.DeviceState, error) {
	response, err := b.sendCommand(ctx, &models.SendCommandInput{
		Command: 0x6a,
		Payload: []byte{12, 0, 187, 0, 6, 128, 0, 0, 2, 0, 17, 1, 43, 126, 0, 0},
		Mac:     mac,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to send the command to get states",
			slog.String("device", mac),
			slog.Any("err", err))
		return nil, err
	}

	if uint16(response.Payload[0x22])|(uint16(response.Payload[0x23])<<8) != 0 {
		slog.ErrorContext(ctx, "Checksum is incorrect",
			slog.String("device", mac),
			slog.Any("payload", response.Payload))
		return nil, models.ErrorInvalidResultPacket
	}

	readDeviceAuthReturn, err := b.cache.ReadDeviceAuth(ctx, &modelsRepo.ReadDeviceAuthInput{Mac: mac})
	if err != nil {
		slog.ErrorContext(ctx, "device not found",
			slog.String("device", mac),
			slog.Any("err", err))
		return nil, err
	}

	if len(response.Payload) >= 0x38 {
		response.Payload = response.Payload[0x38:]
	} else {
		slog.ErrorContext(ctx, "response is too short",
			slog.String("device", mac),
			slog.Any("payload", response.Payload))
		return nil, models.ErrorInvalidResultPacketLength
	}

	response.Payload, err = coder.Decrypt(readDeviceAuthReturn.Auth.Key, readDeviceAuthReturn.Auth.Iv, response.Payload)
	if err != nil {
		slog.ErrorContext(ctx, "failed to decrypt the response",
			slog.String("device", mac),
			slog.Any("err", err))
		return nil, err
	}

	if response.Payload[4] != 0x07 {
		slog.ErrorContext(ctx, "it is not a result packet",
			slog.String("device", mac),
			slog.Any("payload", response.Payload))
		return nil, models.ErrorInvalidResultPacket
	}

	if response.Payload[0] != 0x19 {
		slog.ErrorContext(ctx, "the length of the packet is incorrect. Must be 25",
			slog.String("device", mac),
			slog.Any("payload", response.Payload))
		return nil, models.ErrorInvalidResultPacketLength
	}

	response.Payload = response.Payload[2:]

	raw := models.DeviceStatusRaw{
		UpdatedAt:          time.Now(),
		Temperature:        float32(8+(response.Payload[10]>>3)) + 0.5*float32(response.Payload[12]>>7),
		Power:              response.Payload[18] >> 5 & 0b00000001,
		FixationVertical:   response.Payload[10] & 0b00000111,
		Mode:               response.Payload[15] >> 5 & 0b00001111,
		Sleep:              response.Payload[15] >> 2 & 0b00000001,
		Display:            response.Payload[20] >> 4 & 0b00000001,
		Mildew:             response.Payload[20] >> 3 & 0b00000001,
		Health:             response.Payload[18] >> 1 & 0b00000001,
		FixationHorizontal: response.Payload[10] & 0b00000111,
		FanSpeed:           response.Payload[13] >> 5 & 0b00000111,
		IFeel:              response.Payload[15] >> 3 & 0b00000001,
		Mute:               response.Payload[14] >> 7 & 0b00000001,
		Turbo:              response.Payload[14] >> 6 & 0b00000001,
		Clean:              response.Payload[18] >> 2 & 0b00000001,
	}

	if raw.Temperature < 16.0 {
		slog.ErrorContext(ctx, "wrong temperature, skip package",
			slog.String("device", mac),
			slog.Any("temperature", raw.Temperature))
		return nil, models.ErrorInvalidResultPacketLength
	}

	readDeviceConfigReturn, err := b.cache.ReadDeviceConfig(ctx, &modelsRepo.ReadDeviceConfigInput{Mac: mac})
	if err != nil {
		slog.ErrorContext(ctx, "failed to read device config",
			slog.Any("err", err),
			slog.String("device", mac))
		return nil, err
	}

	return &models.DeviceState{
		Status: raw.ConvertToDeviceStatusHA(readDeviceConfigReturn.Config.InvertDisplay),
		Raw:    &raw,
	}, nil
}

func (b *localBackend) ApplyState(ctx context.Context, mac string, input *models.UpdateDeviceStatesInput) error {
	readDeviceStatusRawReturn, err := b.cache.ReadDeviceStatusRaw(ctx, &modelsRepo.ReadDeviceStatusRawInput{Mac: mac})
	if err != nil {
		slog.ErrorContext(ctx, "failed to read device raw status",
			slog.Any("err", err),
			slog.String("device", mac))
		return err
	}

	var verticalFixation byte
	if input.SwingMode != nil {
		key, ok := models.VerticalFixationStatusesInvert[*input.SwingMode]
		if !ok {
			slog.ErrorContext(ctx, "Invalid parameter Swing mode",
				slog.String("device", mac),
				slog.Any("input", *input.SwingMode))
			return models.ErrorInvalidParameterSwingMode
		}
		verticalFixation = byte(key)
	} else {
		verticalFixation = readDeviceStatusRawReturn.Status.FixationVertical
	}

	var temperature, temperature05 int
	if input.Temperature != nil {
		if *input.Temperature > 32 || *input.Temperature < 16 {
			slog.ErrorContext(ctx, "Invalid parameter temperature",
				slog.String("device", mac),
				slog.Any("input", *input.Temperature))
			return models.ErrorInvalidParameterTemperature
		}

		temperature = int(*input.Temperature) - 8
		if int(*input.Temperature*10)%(int(*input.Temperature)*10) == 5 {
			temperature05 = 1
		}
	} else {
		switch {
		case readDeviceStatusRawReturn.Status.Temperature < 16:
			temperature = 16 - 8
		case readDeviceStatusRawReturn.Status.Temperature > 32:
			temperature = 32 - 8
		default:
			temperature = int(readDeviceStatusRawReturn.Status.Temperature) - 8
			if readDeviceStatusRawReturn.Status.Temperature-float32(int(readDeviceStatusRawReturn.Status.Temperature)) != 0 {
				temperature05 = 1
			}
		}
	}

	var fanMode, turbo, mute byte
	if input.FanMode != nil {
		switch *input.FanMode {
		case "mute":
			mute = models.StatusOn
		case "turbo":
			turbo = models.StatusOn
		default:
			key, ok := models.FanStatusesInvert[*input.FanMode]
			if !ok {
				slog.ErrorContext(ctx, "Invalid parameter fan mode",
					slog.String("device", mac),
					slog.Any("input", *input.FanMode))
				return models.ErrorInvalidParameterFanMode
			}
			fanMode = byte(key)
		}
	} else {
		fanMode = readDeviceStatusRawReturn.Status.FanSpeed
		mute = readDeviceStatusRawReturn.Status.Mute
		turbo = readDeviceStatusRawReturn.Status.Turbo
	}

	var mildew byte
	if input.IsMildewOn != nil {
		mildew = models.HassOnOffToByte(*input.IsMildewOn)
	} else {
		mildew = readDeviceStatusRawReturn.Status.Mildew
	}

	var clean byte
	if input.IsCleanOn != nil {
		clean = models.HassOnOffToByte(*input.IsCleanOn)
	} else {
		clean = readDeviceStatusRawReturn.Status.Clean
	}

	var health byte
	if input.IsHealthOn != nil {
		health = models.HassOnOffToByte(*input.IsHealthOn)
	} else {
		health = readDeviceStatusRawReturn.Status.Health
	}

	var displaySwitch byte
	if input.IsDisplayOn != nil {
		readDeviceConfigReturn, err := b.cache.ReadDeviceConfig(ctx, &modelsRepo.ReadDeviceConfigInput{Mac: mac})
		if err != nil {
			slog.ErrorContext(ctx, "failed to read device config",
				slog.Any("err", err),
				slog.String("device", mac))
			return err
		}
		displaySwitch = models.HassDisplayToByte(*input.IsDisplayOn, readDeviceConfigReturn.Config.InvertDisplay)
	} else {
		displaySwitch = readDeviceStatusRawReturn.Status.Display
	}

	var mode, power byte
	if input.Mode != nil {
		if *input.Mode == "off" {
			mode = readDeviceStatusRawReturn.Status.Mode
			power = models.StatusOff
		} else {
			key, ok := models.ModeStatusesInvert[*input.Mode]
			if !ok {
				slog.ErrorContext(ctx, "Invalid parameter mode",
					slog.String("device", mac),
					slog.Any("input", *input.Mode))
				return models.ErrorInvalidParameterMode
			}
			mode = byte(key)
			power = models.StatusOn
		}
	} else {
		power = readDeviceStatusRawReturn.Status.Power
		mode = readDeviceStatusRawReturn.Status.Mode
	}

	var payload [23]byte
	payload[0] = 0xbb
	payload[1] = 0x00
	payload[2] = 0x06
	payload[3] = 0x80
	payload[4] = 0x00
	payload[5] = 0x00
	payload[6] = 0x0f
	payload[7] = 0x00
	payload[8] = 0x01
	payload[9] = 0x01
	payload[10] = 0b00000000 | byte(temperature)<<3 | verticalFixation
	payload[11] = 0b00000000 | readDeviceStatusRawReturn.Status.FixationHorizontal<<5
	payload[12] = 0b00001111 | byte(temperature05)<<7
	payload[13] = 0b00000000 | fanMode<<5
	payload[14] = 0b00000000 | turbo<<6 | mute<<7
	payload[15] = 0b00000000 | mode<<5 | readDeviceStatusRawReturn.Status.Sleep<<2
	payload[16] = 0b00000000
	payload[17] = 0x00
	payload[18] = 0b00000000 | power<<5 | health<<1 | clean<<2
	payload[19] = 0x00
	payload[20] = 0b00000000 | displaySwitch<<4 | mildew<<3
	payload[21] = 0b00000000
	payload[22] = 0b00000000

	var payloadChecksum [32]byte
	payloadChecksum[0] = byte(len(payload) + 2)
	copy(payloadChecksum[2:], payload[:])

	var checksum int
	for i := 0; i < len(payload); i += 2 {
		checksum += int(payload[i])<<8 + int(append(payload[:], byte(0))[i+1])
	}
	checksum = (checksum >> 16) + (checksum & 0xFFFF)
	checksum = ^checksum & 0xFFFF

	payloadChecksum[len(payload)+2] = byte((checksum >> 8) & 0xFF)
	payloadChecksum[len(payload)+3] = byte(checksum & 0xFF)

	_, err = b.sendCommand(ctx, &models.SendCommandInput{
		Command: 0x6a,
		Payload: payloadChecksum[:],
		Mac:     mac,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to send a set command",
			slog.Any("err", err),
			slog.String("device", mac))
		return err
	}

	return nil
}

func (b *localBackend) sendCommand(ctx context.Context, input *models.SendCommandInput) (*models.SendCommandReturn, error) {
	readDeviceAuthReturn, err := b.cache.ReadDeviceAuth(ctx, &modelsRepo.ReadDeviceAuthInput{Mac: input.Mac})
	if err != nil {
		slog.ErrorContext(ctx, "device not found",
			slog.Any("err", err),
			slog.String("device", input.Mac))
		return nil, err
	}

	auth := readDeviceAuthReturn.Auth
	auth.LastMessageId = (auth.LastMessageId + 1) & 0xffff

	macByteSlice := make([]byte, 0, len(input.Mac)/2)
	for i := 0; i < len(input.Mac); i += 2 {
		val, parseErr := strconv.ParseUint(input.Mac[i:i+2], 16, 8)
		if parseErr != nil {
			slog.ErrorContext(ctx, "mac address is not correct",
				slog.Any("err", parseErr),
				slog.String("device", input.Mac))
			return nil, parseErr
		}
		macByteSlice = append(macByteSlice, byte(val))
	}

	var packet [0x38]byte
	packet[0x00] = 0x5a
	packet[0x01] = 0xa5
	packet[0x02] = 0xaa
	packet[0x03] = 0x55
	packet[0x04] = 0x5a
	packet[0x05] = 0xa5
	packet[0x06] = 0xaa
	packet[0x07] = 0x55
	packet[0x24] = 0x2a
	packet[0x25] = 0x4e
	packet[0x26] = input.Command
	packet[0x28] = byte(auth.LastMessageId & 0xff)
	packet[0x29] = byte(auth.LastMessageId >> 8)
	packet[0x2a] = macByteSlice[0]
	packet[0x2b] = macByteSlice[1]
	packet[0x2c] = macByteSlice[2]
	packet[0x2d] = macByteSlice[3]
	packet[0x2e] = macByteSlice[4]
	packet[0x2f] = macByteSlice[5]
	packet[0x30] = auth.Id[0]
	packet[0x31] = auth.Id[1]
	packet[0x32] = auth.Id[2]
	packet[0x33] = auth.Id[3]

	checksum := 0xbeaf
	for i := range input.Payload {
		checksum += int(input.Payload[i])
		checksum &= 0xffff
	}

	input.Payload, err = coder.Encrypt(auth.Key, auth.Iv, input.Payload)
	if err != nil {
		slog.ErrorContext(ctx, "failed to encrypt payload",
			slog.Any("err", err),
			slog.String("device", input.Mac))
		return nil, err
	}

	packet[0x34] = byte(checksum & 0xff)
	packet[0x35] = byte(checksum >> 8)

	packetSlice := packet[:]
	packetSlice = append(packetSlice, input.Payload...)

	checksum = 0xbeaf
	for i := range packetSlice {
		checksum += int(packetSlice[i])
		checksum &= 0xffff
	}
	packetSlice[0x20] = byte(checksum & 0xff)
	packetSlice[0x21] = byte(checksum >> 8)

	err = b.cache.UpsertDeviceAuth(ctx, &modelsRepo.UpsertDeviceAuthInput{
		Mac:  input.Mac,
		Auth: auth,
	})
	if err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "packet",
		slog.String("device", input.Mac),
		slog.Any("input", packetSlice))

	readDeviceConfigReturn, err := b.cache.ReadDeviceConfig(ctx, &modelsRepo.ReadDeviceConfigInput{Mac: input.Mac})
	if err != nil {
		slog.ErrorContext(ctx, "failed to read device config",
			slog.Any("err", err),
			slog.String("device", input.Mac))
		return nil, err
	}

	sendCommandReturn, err := b.webClient.SendCommand(ctx, &modelsWeb.SendCommandInput{
		Payload: packetSlice,
		Ip:      readDeviceConfigReturn.Config.Ip,
		Port:    readDeviceConfigReturn.Config.Port,
	})
	if err != nil {
		slog.ErrorContext(ctx, "failed to send a command",
			slog.Any("err", err),
			slog.String("device", input.Mac))
		return nil, err
	}

	return &models.SendCommandReturn{Payload: sendCommandReturn.Payload}, nil
}

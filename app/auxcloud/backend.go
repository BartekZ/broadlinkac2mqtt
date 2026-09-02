package auxcloud

import (
	"context"
	"log/slog"
	"time"

	"github.com/ArtemVladimirov/broadlinkac2mqtt/app"
	"github.com/ArtemVladimirov/broadlinkac2mqtt/app/service/models"
)

type Backend struct {
	client *Client
}

func NewBackend(client *Client) app.DeviceBackend {
	return &Backend{client: client}
}

func (b *Backend) MinRequestGap() time.Duration {
	return minRequestGap
}

func (b *Backend) Capabilities(_ context.Context, mac string) models.DeviceCapabilities {
	return b.client.Capabilities(mac)
}

func (b *Backend) Authenticate(ctx context.Context, mac string) error {
	if err := b.client.ensureLogin(ctx); err != nil {
		return err
	}
	if _, ok := b.client.Device(mac); !ok {
		if _, err := b.client.DiscoverDevices(ctx); err != nil {
			return err
		}
	}
	if _, ok := b.client.Device(mac); !ok {
		return ErrDeviceNotFound
	}
	if _, err := b.client.ProbeCapabilities(ctx, mac); err != nil {
		slog.WarnContext(ctx, "failed to probe aux cloud capabilities",
			slog.String("device", mac),
			slog.Any("err", err))
	}
	return nil
}

func (b *Backend) ReadState(ctx context.Context, mac string) (*models.DeviceState, error) {
	params, err := b.client.GetDeviceParams(ctx, mac, nil)
	if err != nil {
		return nil, err
	}

	state := StateFromParams(params)
	if state.AmbientTemp == nil {
		special, specialErr := b.client.GetDeviceParams(ctx, mac, []string{"mode"})
		if specialErr != nil {
			slog.DebugContext(ctx, "aux cloud ambient temperature is unavailable",
				slog.String("device", mac),
				slog.Any("err", specialErr))
		} else if ambient, ok := special["envtemp"]; ok {
			params["envtemp"] = ambient
			state = StateFromParams(params)
		}
	}

	return &state, nil
}

func (b *Backend) ReadAmbient(ctx context.Context, mac string) (*float32, error) {
	params, err := b.client.GetDeviceParams(ctx, mac, []string{"mode"})
	if err != nil {
		return nil, err
	}
	state := StateFromParams(params)
	return state.AmbientTemp, nil
}

func (b *Backend) ApplyState(ctx context.Context, mac string, input *models.UpdateDeviceStatesInput) error {
	params := ParamsFromUpdate(input)
	if len(params) == 0 {
		return nil
	}
	return b.client.SetDeviceParams(ctx, mac, params)
}

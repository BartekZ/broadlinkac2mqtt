package subscriber

import (
	"context"
	"log/slog"
	"time"

	"github.com/ArtemVladimirov/broadlinkac2mqtt/app"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const unsubscribeTimeout = time.Second

func deviceCommandTopics(mac string, topicPrefix string) []string {
	prefix := topicPrefix + "/" + mac
	return []string{
		prefix + "/fan_mode/set",
		prefix + "/swing_mode/set",
		prefix + "/mode/set",
		prefix + "/temp/set",
		prefix + "/display/switch/set",
		prefix + "/mildew/switch/set",
		prefix + "/clean/switch/set",
		prefix + "/health/switch/set",
	}
}

func Routers(ctx context.Context, mac string, topicPrefix string, client mqtt.Client, handler app.MqttSubscriber) {
	prefix := topicPrefix + "/" + mac

	if token := client.Subscribe(prefix+"/fan_mode/set", 0, handler.UpdateFanModeCommandTopic(ctx)); token.Wait() && token.Error() != nil {
		slog.ErrorContext(ctx, "failed to subscribe on topic", slog.Any("err", token.Error()))
	}
	if token := client.Subscribe(prefix+"/swing_mode/set", 0, handler.UpdateSwingModeCommandTopic(ctx)); token.Wait() && token.Error() != nil {
		slog.ErrorContext(ctx, "failed to subscribe on topic", slog.Any("err", token.Error()))
	}
	if token := client.Subscribe(prefix+"/mode/set", 0, handler.UpdateModeCommandTopic(ctx)); token.Wait() && token.Error() != nil {
		slog.ErrorContext(ctx, "failed to subscribe on topic", slog.Any("err", token.Error()))
	}
	if token := client.Subscribe(prefix+"/temp/set", 0, handler.UpdateTemperatureCommandTopic(ctx)); token.Wait() && token.Error() != nil {
		slog.ErrorContext(ctx, "failed to subscribe on topic", slog.Any("err", token.Error()))
	}
	if token := client.Subscribe(prefix+"/display/switch/set", 0, handler.UpdateDisplaySwitchCommandTopic(ctx)); token.Wait() && token.Error() != nil {
		slog.ErrorContext(ctx, "failed to subscribe on topic", slog.Any("err", token.Error()))
	}
	if token := client.Subscribe(prefix+"/mildew/switch/set", 0, handler.UpdateMildewSwitchCommandTopic(ctx)); token.Wait() && token.Error() != nil {
		slog.ErrorContext(ctx, "failed to subscribe on topic", slog.Any("err", token.Error()))
	}
	if token := client.Subscribe(prefix+"/clean/switch/set", 0, handler.UpdateCleanSwitchCommandTopic(ctx)); token.Wait() && token.Error() != nil {
		slog.ErrorContext(ctx, "failed to subscribe on topic", slog.Any("err", token.Error()))
	}
	if token := client.Subscribe(prefix+"/health/switch/set", 0, handler.UpdateHealthSwitchCommandTopic(ctx)); token.Wait() && token.Error() != nil {
		slog.ErrorContext(ctx, "failed to subscribe on topic", slog.Any("err", token.Error()))
	}
}

func Unsubscribe(ctx context.Context, mac string, topicPrefix string, client mqtt.Client) {
	topics := deviceCommandTopics(mac, topicPrefix)
	token := client.Unsubscribe(topics...)
	if token.WaitTimeout(unsubscribeTimeout) && token.Error() != nil {
		slog.ErrorContext(ctx, "failed to unsubscribe from topics",
			slog.Any("err", token.Error()),
			slog.String("device", mac))
	}
}

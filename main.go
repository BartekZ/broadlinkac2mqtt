package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ArtemVladimirov/broadlinkac2mqtt/app"
	"github.com/ArtemVladimirov/broadlinkac2mqtt/app/auxcloud"
	"github.com/ArtemVladimirov/broadlinkac2mqtt/app/mqtt"
	workspaceMqttModels "github.com/ArtemVladimirov/broadlinkac2mqtt/app/mqtt/models"
	workspaceMqttSender "github.com/ArtemVladimirov/broadlinkac2mqtt/app/mqtt/publisher"
	workspaceMqttReceiver "github.com/ArtemVladimirov/broadlinkac2mqtt/app/mqtt/subscriber"
	workspaceCache "github.com/ArtemVladimirov/broadlinkac2mqtt/app/repository/cache"
	workspaceService "github.com/ArtemVladimirov/broadlinkac2mqtt/app/service"
	workspaceServiceModels "github.com/ArtemVladimirov/broadlinkac2mqtt/app/service/models"
	workspaceWebClient "github.com/ArtemVladimirov/broadlinkac2mqtt/app/webClient"
	"github.com/ArtemVladimirov/broadlinkac2mqtt/config"
	paho "github.com/eclipse/paho.mqtt.golang"
	"golang.org/x/sync/errgroup"
)

const (
	monitorShutdownTimeout = 3 * time.Second
	offlinePublishTimeout  = 2 * time.Second
	mqttDisconnectQuiesce  = 250
	mqttUnsubscribeTimeout = time.Second
)

type App struct {
	devices            []workspaceServiceModels.DeviceConfig
	autoDiscoveryTopic *string
	topicPrefix        string
	logLevel           string
	wsMqttReceiver     app.MqttSubscriber
	wsService          app.Service
	client             paho.Client
}

func NewApp() (*App, error) {
	// Configuration
	cfg, err := config.NewConfig()
	if err != nil {
		return nil, err
	}

	// MQTT
	mqttConfig := workspaceMqttModels.ConfigMqtt{
		Broker:                   cfg.Mqtt.Broker,
		User:                     cfg.Mqtt.User,
		Password:                 cfg.Mqtt.Password,
		ClientId:                 cfg.Mqtt.ClientId,
		TopicPrefix:              cfg.Mqtt.TopicPrefix,
		AutoDiscoveryTopic:       cfg.Mqtt.AutoDiscoveryTopic,
		AutoDiscoveryTopicRetain: cfg.Mqtt.AutoDiscoveryTopicRetain,
	}

	opts, err := mqtt.NewMqttConfig(cfg.Mqtt)
	if err != nil {
		return nil, err
	}

	client := paho.NewClient(opts)

	var cloudBackend app.DeviceBackend
	var cloudClient *auxcloud.Client
	if cfg.Cloud.Enabled {
		cloudClient, err = auxcloud.NewClient(auxcloud.Config{
			Email:    cfg.Cloud.Email,
			Password: cfg.Cloud.Password,
			Region:   cfg.Cloud.Region,
		})
		if err != nil {
			return nil, err
		}
		if err = cloudClient.Login(context.Background()); err != nil {
			return nil, err
		}
		cloudBackend = auxcloud.NewBackend(cloudClient)
	}

	// Configure MQTT Sender Layer
	mqttSender := workspaceMqttSender.NewMqttSender(
		mqttConfig,
		client,
	)

	// Configure Service Layer
	service := workspaceService.NewService(
		cfg.Mqtt.TopicPrefix,
		cfg.Service.UpdateInterval,
		mqttSender,
		workspaceWebClient.NewWebClient(),
		workspaceCache.NewCache(),
		cloudBackend,
	)
	// Configure MQTT Receiver Layer
	mqttReceiver := workspaceMqttReceiver.NewMqttReceiver(
		service,
		mqttConfig,
	)

	devices, err := loadDevices(context.Background(), cfg, cloudClient)
	if err != nil {
		return nil, err
	}

	application := &App{
		wsMqttReceiver:     mqttReceiver,
		client:             client,
		devices:            devices,
		wsService:          service,
		topicPrefix:        cfg.Mqtt.TopicPrefix,
		autoDiscoveryTopic: cfg.Mqtt.AutoDiscoveryTopic,
		logLevel:           cfg.Service.LogLevel,
	}

	return application, nil
}

func (app *App) Run(ctx context.Context) error {
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	// Run MQTT
	if token := app.client.Connect(); token.Wait() && token.Error() != nil {
		err := token.Error()
		slog.ErrorContext(ctx, "failed to connect mqtt",
			slog.Any("err", err))
		return err
	}
	defer app.client.Disconnect(mqttDisconnectQuiesce)

	if app.autoDiscoveryTopic != nil {
		if token := app.client.Subscribe(*app.autoDiscoveryTopic+"/status", 0, app.wsMqttReceiver.GetStatesOnHomeAssistantRestart(runCtx)); token.Wait() && token.Error() != nil {
			err := token.Error()
			slog.ErrorContext(ctx, "failed to subscribe on LWT",
				slog.Any("err", err))
			return err
		}
	}

	// Create Device
	for _, device := range app.devices {
		err := app.wsService.CreateDevice(ctx, &workspaceServiceModels.CreateDeviceInput{
			Config: workspaceServiceModels.DeviceConfig{
				Mac:             device.Mac,
				Ip:              device.Ip,
				Name:            device.Name,
				Port:            device.Port,
				TemperatureUnit: device.TemperatureUnit,
				InvertDisplay:   device.InvertDisplay,
				Backend:         device.Backend,
				CloudEndpointID: device.CloudEndpointID,
				CloudProductID:  device.CloudProductID,
			}})
		if err != nil {
			slog.ErrorContext(ctx, "failed to create the device",
				slog.Any("err", err))
			return err
		}
	}

	var monitors sync.WaitGroup
	for _, device := range app.devices {
		monitors.Add(1)
		go func() {
			defer monitors.Done()

			for {
				if runCtx.Err() != nil {
					return
				}
				err := app.wsService.AuthDevice(runCtx, &workspaceServiceModels.AuthDeviceInput{Mac: device.Mac})
				if err == nil {
					break
				}
				slog.ErrorContext(runCtx, "failed to Auth device "+device.Mac+". Reconnect in 3 seconds...",
					slog.Any("err", err))
				select {
				case <-runCtx.Done():
					return
				case <-time.After(time.Second * 3):
				}
			}

			// Subscribe on MQTT handlers
			workspaceMqttReceiver.Routers(runCtx, device.Mac, app.topicPrefix, app.client, app.wsMqttReceiver)

			// Publish Discovery Topic
			if app.autoDiscoveryTopic != nil {
				err := app.wsService.PublishDiscoveryTopic(runCtx, &workspaceServiceModels.PublishDiscoveryTopicInput{Device: device})
				if err != nil {
					slog.ErrorContext(runCtx, "failed to publish discovery topic",
						slog.String("device", device.Mac),
						slog.Any("err", err))
				}
			}

			err := app.wsService.StartDeviceMonitoring(runCtx, &workspaceServiceModels.StartDeviceMonitoringInput{Mac: device.Mac})
			if err != nil {
				slog.ErrorContext(runCtx, "device monitoring stopped",
					slog.String("device", device.Mac),
					slog.Any("err", err))
			}
		}()
	}

	// Graceful shutdown
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)

	select {
	case killSignal := <-interrupt:
		switch killSignal {
		case syscall.SIGQUIT:
			slog.InfoContext(ctx, "Got SIGQUIT...")
		case syscall.SIGTERM:
			slog.InfoContext(ctx, "Got SIGTERM...")
		case syscall.SIGINT:
			slog.InfoContext(ctx, "Got SIGINT...")
		default:
			slog.InfoContext(ctx, "Undefined killSignal...")
		}
	case <-runCtx.Done():
		slog.InfoContext(ctx, "run context cancelled...")
	}
	signal.Stop(interrupt)

	app.unsubscribeMQTT(ctx)
	cancelRun()
	if !waitWaitGroup(&monitors, monitorShutdownTimeout) {
		slog.WarnContext(ctx, "timed out waiting for device monitors to stop")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.WithoutCancel(ctx), offlinePublishTimeout)
	defer cancelShutdown()

	g, gCtx := errgroup.WithContext(shutdownCtx)
	for _, device := range app.devices {
		g.Go(func() error {
			err := app.wsService.UpdateDeviceAvailability(gCtx, &workspaceServiceModels.UpdateDeviceAvailabilityInput{
				Mac:          device.Mac,
				Availability: "offline",
			})
			if err != nil {
				slog.ErrorContext(gCtx, "failed to update availability",
					slog.String("device", device.Mac),
					slog.Any("err", err))
				return err
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	return nil
}

func (app *App) unsubscribeMQTT(ctx context.Context) {
	if !app.client.IsConnected() {
		return
	}

	if app.autoDiscoveryTopic != nil {
		topic := *app.autoDiscoveryTopic + "/status"
		if token := app.client.Unsubscribe(topic); token.WaitTimeout(mqttUnsubscribeTimeout) && token.Error() != nil {
			slog.ErrorContext(ctx, "failed to unsubscribe from LWT", slog.Any("err", token.Error()))
		}
	}

	for _, device := range app.devices {
		workspaceMqttReceiver.Unsubscribe(ctx, device.Mac, app.topicPrefix, app.client)
	}
}

func waitWaitGroup(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logLevel := &slog.LevelVar{}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     logLevel,
	})))

	application, err := NewApp()
	if err != nil {
		slog.ErrorContext(ctx, "failed to get a new App", slog.Any("err", err))
		return
	}

	switch application.logLevel {
	case "error":
		logLevel.Set(slog.LevelError)
	case "debug":
		logLevel.Set(slog.LevelDebug)
	case "disabled":
		slog.SetDefault(slog.New(slog.DiscardHandler))
	case "info":
		logLevel.Set(slog.LevelInfo)
	default:
		logLevel.Set(slog.LevelError)
	}

	// Run
	err = application.Run(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "failed to run app", slog.Any("err", err))
		return
	}
}

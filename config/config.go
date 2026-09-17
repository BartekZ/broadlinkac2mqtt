package config

import (
	"errors"
	"log/slog"
	"os"
	"strings"

	"github.com/ilyakaznacheev/cleanenv"
)

type (
	Config struct {
		Service Service   `yaml:"service" json:"service"`
		Mqtt    Mqtt      `yaml:"mqtt" json:"mqtt"`
		Cloud   Cloud     `yaml:"cloud" json:"cloud"`
		Devices []Devices `yaml:"devices" json:"devices"`
	}

	Service struct {
		UpdateInterval int    `env-default:"10"    yaml:"update_interval" json:"update_interval"`
		LogLevel       string `env-default:"error" yaml:"log_level" json:"log_level"`
	}

	Mqtt struct {
		Broker                   string  `env-required:"true" yaml:"broker" json:"broker"`
		User                     *string `yaml:"user" json:"user"`
		Password                 *string `yaml:"password" json:"password"`
		ClientId                 string  `env-default:"broadlinkac" yaml:"client_id" json:"client_id"`
		TopicPrefix              string  `env-default:"airac" yaml:"topic_prefix" json:"topic_prefix"`
		AutoDiscoveryTopic       *string `yaml:"auto_discovery_topic" json:"auto_discovery_topic"`
		AutoDiscoveryTopicRetain bool    `env-default:"true" yaml:"auto_discovery_topic_retain" json:"auto_discovery_topic_retain"`
		CertificateAuthority     *string `yaml:"certificate_authority" json:"certificate_authority"`
		SkipCertCnCheck          bool    `env-default:"true" yaml:"skip_cert_cn_check" json:"skip_cert_cn_check"`
		CertificateClient        *string `yaml:"certificate_client" json:"certificate_client"`
		KeyClient                *string `yaml:"key-client" json:"key_client"`
	}

	Cloud struct {
		Enabled         bool     `yaml:"enabled" json:"enabled"`
		Email           string   `yaml:"email" json:"email"`
		Password        string   `yaml:"password" json:"password"`
		Region          string   `env-default:"eu" yaml:"region" json:"region"`
		AutoDiscover    *bool    `yaml:"auto_discover" json:"auto_discover"`
		HiddenDevices   []string `yaml:"hidden_devices" json:"hidden_devices"`
		TemperatureUnit string   `env-default:"C" yaml:"temperature_unit" json:"temperature_unit"`
	}

	Devices struct {
		Ip   string `yaml:"ip" json:"ip"`
		Mac  string `yaml:"mac" json:"mac"`
		Name string `yaml:"name" json:"name"`
		Port uint16 `yaml:"port" json:"port"`
		// TemperatureUnit defines the temperature unit of the device, C or F.
		// If this is not set, the temperature unit is Celsius.
		TemperatureUnit string `env-default:"C" yaml:"temperature_unit" json:"temperature_unit"` // BUG cleanenv env-default is not working
		// InvertDisplay flips the display ON/OFF protocol mapping.
		// Default (false): byte 0 = ON, byte 1 = OFF.
		InvertDisplay bool   `env-default:"false" yaml:"invert_display" json:"invert_display"`
		Transport     string `yaml:"transport" json:"transport"`
		DeviceID      string `yaml:"device_id" json:"device_id"`
	}
)

func (c Cloud) AutoDiscoverEnabled() bool {
	if c.AutoDiscover == nil {
		return true
	}
	return *c.AutoDiscover
}

func (c Cloud) Validate() error {
	if !c.Enabled {
		return nil
	}
	if strings.TrimSpace(c.Email) == "" || c.Password == "" {
		return errors.New("cloud email and password are required")
	}
	region := strings.ToLower(strings.TrimSpace(c.Region))
	if region == "" {
		region = "eu"
	}
	switch region {
	case "eu", "usa", "cn":
	default:
		return errors.New("cloud region must be eu, usa, or cn")
	}
	unit := strings.ToUpper(strings.TrimSpace(c.TemperatureUnit))
	if unit == "" {
		unit = "C"
	}
	if unit != "C" && unit != "F" {
		return errors.New("cloud temperature_unit must be C or F")
	}
	return nil
}

// NewConfig returns app config.
func NewConfig() (*Config, error) {
	cfg := &Config{}

	files := [...]string{
		"./config/config.yml",
		"./config/config.json",
		"/data/options.json", // hassio
	}

	for i := range files {
		if _, err := os.Stat(files[i]); err == nil {
			err = cleanenv.ReadConfig(files[i], cfg)
			if err != nil {
				slog.Error("failed to read config", slog.Any("err", err))
				return nil, err
			}
			if err = cfg.Cloud.Validate(); err != nil {
				return nil, err
			}
			return cfg, nil
		}
	}

	return nil, errors.New("config file is not found")
}

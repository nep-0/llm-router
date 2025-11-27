package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	Port   int64  `mapstructure:"port"`
	APIKey string `mapstructure:"api_key"`

	ErrorPenalty   int64 `mapstructure:"error_penalty"`
	RequestPenalty int64 `mapstructure:"request_penalty"`

	Groups    []Group    `mapstructure:"groups"`
	Providers []Provider `mapstructure:"providers"`
}

type Group struct {
	Name   string  `mapstructure:"name"`
	Models []Model `mapstructure:"models"`
}

type Model struct {
	Weight   int64  `mapstructure:"weight"`
	Provider string `mapstructure:"provider"`
	Name     string `mapstructure:"name"`
}

type Provider struct {
	Name    string   `mapstructure:"name"`
	BaseURL string   `mapstructure:"base_url"`
	APIKeys []string `mapstructure:"api_keys"`
}

func LoadConfig(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, err
	}
	return &config, nil
}

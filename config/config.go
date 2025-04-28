package config

import (
	"log/slog"
	"os"
	"path"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	ExamDates             string        `mapstructure:"exam_dates"`
	Addresses             string        `mapstructure:"addresses"`
	ExamType              int           `mapstructure:"exam_type"`
	LogLevel              string        `mapstructure:"log_level"`
	BrowsersCount         int           `mapstructure:"browsers_count"`
	DefaultTimeout        time.Duration `mapstructure:"default_timeout"`
	IntervalBetweenChecks time.Duration `mapstructure:"interval_between_checks"`
	TtlForFoundTask       time.Duration `mapstructure:"ttl_for_found_task"`
	NtfyTopic             string        `mapstructure:"ntfy_topic"`
}

func MustLoadConfig() *Config {
	viper.AddConfigPath(path.Join("."))
	viper.SetConfigName("config")
	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	if err != nil {
		slog.Error("can't initialize config file.", slog.String("err", err.Error()))
		os.Exit(1)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		slog.Error("error unmarshalling viper config.", slog.String("err", err.Error()))
		os.Exit(1)
	}

	return &cfg
}

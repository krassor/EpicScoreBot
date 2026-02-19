package config

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"

	"github.com/ilyakaznacheev/cleanenv"
	"gopkg.in/yaml.v3"
)

func MustLoad() *Config {
	op := "config.MustLoad()"
	log := slog.With(
		slog.String("op", op),
	)
	defaultConfigPath := "config.yml"

	configPath := fetchConfigPath()

	if configPath == "" {
		log.Warn("config path is empty. Loading default config path",
			slog.String("defaultConfigPath", defaultConfigPath))
		configPath = defaultConfigPath
	}

	return MustLoadPath(configPath)
}

func MustLoadPath(configPath string) *Config {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		log.Fatalf("config file does not exist: %s", configPath)
	}

	var cfg Config

	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		log.Fatalf("cannot read config: %s", err.Error())
	}

	cfg.configPath = configPath
	return &cfg
}

func fetchConfigPath() string {
	op := "config.fetchConfigPath()"
	log := slog.With(
		slog.String("op", op),
	)

	var res string

	flag.StringVar(&res, "config", "", "path to config file")
	flag.Parse()

	if res != "" {
		log.Info("load config path from command line.",
			slog.String("path", res))
		return res
	}
	res = fmt.Sprintf("%s%s",
		os.Getenv("CONFIG_FILEPATH"),
		os.Getenv("CONFIG_FILENAME"))
	log.Info(
		"load config path from env",
		slog.String("CONFIG_FILEPATH", os.Getenv("CONFIG_FILEPATH")),
		slog.String("CONFIG_FILENAME", os.Getenv("CONFIG_FILENAME")),
	)
	return res
}

func (cfg *Config) Write() error {
	bufWrite, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("error config.Write() marshall: %w", err)
	}

	err = os.WriteFile(cfg.configPath, bufWrite, 0775)
	if err != nil {
		return fmt.Errorf("error config.Write() write file: %w", err)
	}
	return nil
}

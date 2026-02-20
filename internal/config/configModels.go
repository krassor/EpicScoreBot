package config

import "time"

type Config struct {
	Env            string           `yaml:"env" env-default:"local"`
	HttpServer     HttpServerConfig `yaml:"httpServer"`
	DBConfig       DBConfig         `yaml:"db" env-required:"true"`
	BotConfig      BotConfig        `yaml:"bot" env-required:"true"`
	ConfigFilePath string           `yaml:"configFilePath" env:"CONFIG_FILEPATH" env-default:""`
	ConfigFileName string           `yaml:"configFileName" env:"CONFIG_FILENAME" env-default:""`
	configPath     string
}

type HttpServerConfig struct {
	Address string        `yaml:"address" env-default:"0.0.0.0"`
	Port    string        `yaml:"port" env-default:"8080"`
	Timeout time.Duration `yaml:"timeout" env-default:"5"`
}

type DBConfig struct {
	Host     string `yaml:"host" env:"DB_HOST" env-default:"localhost"`
	Port     string `yaml:"port" env:"DB_PORT" env-default:"5432"`
	Name     string `yaml:"name" env:"DB_NAME" env-default:"postgres"`
	User     string `yaml:"user" env:"DB_USER" env-default:"user"`
	Password string `yaml:"password" env:"DB_PASSWORD" env-default:"password"`
	Schema   string `yaml:"schema" env:"DB_SCHEMA" env-default:"epic_score"`
}

type BotConfig struct {
	Admins        []string `yaml:"admins" env-default:"admin"`
	TgbotApiToken string   `yaml:"tgbot_apitoken" env:"TGBOT_APITOKEN" env-required:"true"`
}

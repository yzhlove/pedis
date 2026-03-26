package config

import "github.com/jinzhu/configor"

type Config struct {
	ENV          string `default:"dev" json:"env" env:"PEDIS_ENV"`
	CliRedisHost string `default:"127.0.0.1" json:"cli_redis_host" env:"PEDIS_CLI_REDIS_HOST"`
	CliRedisPort string `default:"6379" json:"cli_redis_port" env:"PEDIS_CLI_REDIS_PORT"`
	UnixSocket   string `default:"/tmp/pedis.sock" json:"unix_socket" env:"PEDIS_UNIX_SOCKET"`
	ServerPort   string `default:"6399" json:"server_port" env:"PEDIS_SERVER_PORT"`
}

func New() (*Config, error) {
	cc := &Config{}
	if err := configor.New(&configor.Config{Debug: false}).Load(cc, "config.json"); err != nil {
		return nil, err
	}
	return cc, nil
}

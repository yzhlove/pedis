package config

import "github.com/jinzhu/configor"

type Config struct {
	ENV        string `default:"dev" json:"env" env:"PEDIS_ENV"`
	UnixSocket string `default:"/tmp/pedis.sock" json:"unix_socket" env:"PEDIS_UNIX_SOCKET"`
	ServerPort string `default:"6399" json:"server_port" env:"PEDIS_SERVER_PORT"`
}

func New() (*Config, error) {
	cc := &Config{}
	if err := configor.New(&configor.Config{Debug: false}).Load(cc, "config.json"); err != nil {
		return nil, err
	}
	return cc, nil
}

package config

import (
	"os"

	"github.com/cloudcarver/anclax/lib/conf"
)

type Rw struct {
	// (Required) The DSN (Data Source Name) for postgres database connection. If specified, Host, Port, User, Password, Db, and SSLMode settings will be ignored.
	DSN *string `yaml:"dsn"`

	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Db       string `yaml:"db"`

	// (Optional) The SSL mode for postgres connection, default is "required". Other options are "disable", "verify-ca", "verify-full".
	SSLMode string `yaml:"sslmode"`
}

type Debug struct {
	Port   int  `yaml:"port"`
	Enable bool `yaml:"enable"`
}

type Config struct {
	// (Optional) The host of the anclax server.
	Host string `yaml:"host"`

	// (Optional) The port of the anclax server, default is 8020
	Port int `yaml:"port"`

	// The risingwave configuration
	Rw *Rw `yaml:"rw"`

	Debug Debug `yaml:"debug"`
}

const (
	envPrefix  = "EAPI_"
	configFile = "eventapi.yaml"
)

func NewConfig() (*Config, error) {
	c := &Config{}
	if err := conf.FetchConfig((func() string {
		if _, err := os.Stat(configFile); err != nil {
			return ""
		}
		return configFile
	})(), envPrefix, c); err != nil {
		return nil, err
	}

	return c, nil
}

package conf

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
)

// Config gateway server configuration
type Config struct {
	PostDb *PostDbConfig `yaml:"postdb"`
}

// PostDb postgresql configuration
type PostDbConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Db       string `yaml:"db"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	SslMode  string `yaml:"sslmode"`
	Timeout  int    `yaml:"timeout"`
}

// InitConfig initialize server configuration
func InitConfig(file string) (*Config, error) {
	if len(file) == 0 {
		file = "./server.yml"
	}
	// read server.yml
	bs, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	// init postgresql configuration
	cfg := &Config{}
	if err = yaml.Unmarshal(bs, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

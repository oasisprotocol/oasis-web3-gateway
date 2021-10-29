package conf

import (
	"io/ioutil"
	"time"

	"gopkg.in/yaml.v2"
)

// Config is gateway server configuration.
type Config struct {
	RuntimeID   string `yaml:"runtime_id"`
	NodeAddress string `yaml:"node_address"`

	PostDb  *PostDbConfig  `yaml:"postdb,omitempty"`
	Gateway *GatewayConfig `yaml:"gateway,omitempty"`
}

// PostDbConfig PostDb postgresql configuration.
type PostDbConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Db       string `yaml:"db"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Timeout  int    `yaml:"timeout"`
}

// GatewayConfig is the gateway server configuration.
type GatewayConfig struct {
	// Http is the gateway http endpoint config.
	Http *GatewayHttpConfig `yaml:"http,omitempty"`

	// WS is the gateway websocket endpoint config.
	WS *GatewayWSConfig `yaml:"ws,omitempty"`

	// ChainId defines the Ethereum netwrok chain id.
	ChainId uint32 `toml:",omitempty"`
}

type GatewayHttpConfig struct {
	// Host is the host interface on which to start the HTTP RPC server. Defaults to localhost.
	Host string `yaml:"host"`

	// Port is the port number on which to start the HTTP RPC server. Defaults to 8545.
	Port int `yaml:"port"`

	// Cors are the CORS allowed urls.
	Cors []string `yaml:"cors"`

	// VirtualHosts is the list of virtual hostnames which are allowed on incoming requests.
	VirtualHosts []string `yaml:"virtual_hosts,omitempty"`

	// PathPrefix specifies a path prefix on which http-rpc is to be served. Defaults to '/'.
	PathPrefix string `yaml:"path_prefix"`

	// Timeouts allows for customization of the timeout values used by the HTTP RPC
	// interface.
	Timeouts *HttpTimeouts `yaml:"timeouts,omitempty"`
}

type HttpTimeouts struct {
	Read  *time.Duration `yaml:"read,omitempty"`
	Write *time.Duration `yaml:"write,omitempty"`
	Idle  *time.Duration `yaml:"idle,omitempty"`
}

type GatewayWSConfig struct {
	// Host is the host interface on which to start the HTTP RPC server. Defaults to localhost.
	Host string `yaml:"host"`

	// Port is the port number on which to start the HTTP RPC server. Defaults to 8545.
	Port int `yaml:"port"`

	// PathPrefix specifies a path prefix on which http-rpc is to be served. Defaults to '/'.
	PathPrefix string `yaml:"path_prefix"`

	// Origins is the list of domain to accept websocket requests from.
	Origins []string `yaml:"origins"`

	// Timeouts allows for customization of the timeout values used by the HTTP RPC
	// interface.
	Timeouts *HttpTimeouts `yaml:"timeouts,omitempty"`
}

// InitConfig initializes server configuration
func InitConfig(file string) (*Config, error) {
	// read server.yml
	bs, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}

	// init configuration
	cfg := &Config{}
	if err = yaml.Unmarshal(bs, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

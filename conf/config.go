package conf

import (
	"fmt"
	"time"

	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Config contains the CLI configuration.
type Config struct {
	RuntimeID     string `mapstructure:"runtime_id"`
	NodeAddress   string `mapstructure:"node_address"`
	EnablePruning bool   `mapstructure:"enable_pruning"`
	PruningStep   uint64 `mapstructure:"pruning_step"`

	Log      *LogConfig      `mapstructure:"log"`
	Database *DatabaseConfig `mapstructure:"database"`
	Gateway  *GatewayConfig  `mapstructure:"gateway"`
}

// Validate performs config validation.
func (cfg *Config) Validate() error {
	if cfg.RuntimeID == "" {
		return fmt.Errorf("malformed runtime ID '%s'", cfg.RuntimeID)
	}
	if cfg.NodeAddress == "" {
		return fmt.Errorf("malformed node address '%s'", cfg.NodeAddress)
	}

	if cfg.Log != nil {
		if err := cfg.Log.Validate(); err != nil {
			return err
		}
	}
	if cfg.Database != nil {
		if err := cfg.Database.Validate(); err != nil {
			return fmt.Errorf("database: %w", err)
		}
	}
	if cfg.Gateway != nil {
		if err := cfg.Gateway.Validate(); err != nil {
			return fmt.Errorf("gateway: %w", err)
		}
	}

	return nil
}

// LogConfig contains the logging configuration.
type LogConfig struct {
	Format string `mapstructure:"format"`
	Level  string `mapstructure:"level"`
	File   string `mapstructure:"file"`
}

// Validate validates the logging configuration.
func (cfg *LogConfig) Validate() error {
	var format logging.Format
	if err := format.Set(cfg.Format); err != nil {
		return err
	}
	var level logging.Level
	return level.Set(cfg.Level)
}

// DatabaseConfig is the postgresql database configuration.
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Db       string `mapstructure:"db"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Timeout  int    `mapstructure:"timeout"`
}

// Validate validates the database configuration.
func (cfg *DatabaseConfig) Validate() error {
	// TODO:
	return nil
}

// GatewayConfig is the gateway server configuration.
type GatewayConfig struct {
	// Http is the gateway http endpoint config.
	Http *GatewayHTTPConfig `mapstructure:"http"`

	// WS is the gateway websocket endpoint config.
	WS *GatewayWSConfig `mapstructure:"ws"`

	// ChainId defines the Ethereum netwrok chain id.
	ChainId uint32 `mapstructure:"chain_id"`
}

// Validate validates the gateway configuration.
func (cfg *GatewayConfig) Validate() error {
	// TODO:
	return nil
}

type GatewayHTTPConfig struct {
	// Host is the host interface on which to start the HTTP RPC server. Defaults to localhost.
	Host string `mapstructure:"host"`

	// Port is the port number on which to start the HTTP RPC server. Defaults to 8545.
	Port int `mapstructure:"port"`

	// Cors are the CORS allowed urls.
	Cors []string `mapstructure:"cors"`

	// VirtualHosts is the list of virtual hostnames which are allowed on incoming requests.
	VirtualHosts []string `mapstructure:"virtual_hosts"`

	// PathPrefix specifies a path prefix on which http-rpc is to be served. Defaults to '/'.
	PathPrefix string `mapstructure:"path_prefix"`

	// Timeouts allows for customization of the timeout values used by the HTTP RPC
	// interface.
	Timeouts *HTTPTimeouts `mapsstructure:"timeouts"`
}

type HTTPTimeouts struct {
	Read  *time.Duration `mapstructure:"read"`
	Write *time.Duration `mapstructure:"write"`
	Idle  *time.Duration `mapstructure:"idle"`
}

type GatewayWSConfig struct {
	// Host is the host interface on which to start the HTTP RPC server. Defaults to localhost.
	Host string `mapstructure:"host"`

	// Port is the port number on which to start the HTTP RPC server. Defaults to 8545.
	Port int `mapstructure:"port"`

	// PathPrefix specifies a path prefix on which http-rpc is to be served. Defaults to '/'.
	PathPrefix string `mapstructure:"path_prefix"`

	// Origins is the list of domain to accept websocket requests from.
	Origins []string `mapstructure:"origins"`

	// Timeouts allows for customization of the timeout values used by the HTTP RPC
	// interface.
	Timeouts *HTTPTimeouts `mapstructure:"timeouts"`
}

// InitConfig initializes configuration from file.
func InitConfig(file string) *Config {
	v := viper.New()

	// Set and load config file.
	v.SetConfigFile(file)
	err := v.ReadInConfig()
	cobra.CheckErr(err)

	// Unmarshal config.
	var config Config
	err = v.Unmarshal(&config)
	cobra.CheckErr(err)

	// Validate config.
	err = config.Validate()
	cobra.CheckErr(err)

	return &config
}

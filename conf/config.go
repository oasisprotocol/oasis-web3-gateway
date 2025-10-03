// Package conf provides the configuration for the Oasis Web3 Gateway.
package conf

import (
	"fmt"
	"strings"
	"time"

	"github.com/knadh/koanf"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"

	"github.com/oasisprotocol/oasis-core/go/common/logging"
)

// Config contains the CLI configuration.
type Config struct {
	RuntimeID string `koanf:"runtime_id"`

	NodeAddress   string  `koanf:"node_address"`
	NodeCachePath *string `koanf:"node_cache_path"`

	EnablePruning bool   `koanf:"enable_pruning"`
	PruningStep   uint64 `koanf:"pruning_step"`
	// IndexingStart. Skip indexing before this block number. Use this to avoid trying to index
	// blocks that the node doesn't have data for, such as by skipping them in checkpoint sync.
	IndexingStart uint64 `koanf:"indexing_start"`
	// IndexingEnd. Stop indexing after reaching this block number.
	IndexingEnd     uint64 `koanf:"indexing_end"`
	IndexingDisable bool   `koanf:"indexing_disable"`

	Log      *LogConfig      `koanf:"log"`
	Cache    *CacheConfig    `koanf:"cache"`
	Database *DatabaseConfig `koanf:"database"`
	Gateway  *GatewayConfig  `koanf:"gateway"`

	Gas *GasConfig `koanf:"gas"`

	// ArchiveURI is the URI of an archival web3 gateway instance
	// for servicing historical queries.
	ArchiveURI string `koanf:"archive_uri"`
	// ArchiveHeightMax is the maximum height (inclusive) to query the
	// archive node (ArchiveURI).  If the archive node is configured
	// with it's own SQL database instance, this parameter should not
	// be needed.
	ArchiveHeightMax uint64 `koanf:"archive_height_max"`

	PprofAddr string `koanf:"pprof_addr"`
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
	if cfg.Gas != nil {
		if err := cfg.Gas.Validate(); err != nil {
			return fmt.Errorf("gas: %w", err)
		}
	}

	return nil
}

// LogConfig contains the logging configuration.
type LogConfig struct {
	Format string `koanf:"format"`
	Level  string `koanf:"level"`
	File   string `koanf:"file"`
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

// CacheConfig contains the cache configuration.
type CacheConfig struct {
	// BlockSize is the size of the block cache in BLOCKS.
	BlockSize uint64 `koanf:"block_size"`
	// Metrics enables the cache metrics collection.
	Metrics bool `koanf:"metrics"`
}

// DatabaseConfig is the postgresql database configuration.
type DatabaseConfig struct {
	Host         string `koanf:"host"`
	Port         int    `koanf:"port"`
	DB           string `koanf:"db"`
	User         string `koanf:"user"`
	Password     string `koanf:"password"`
	DialTimeout  int    `koanf:"dial_timeout"`
	ReadTimeout  int    `koanf:"read_timeout"`
	WriteTimeout int    `koanf:"write_timeout"`
	MaxOpenConns int    `koanf:"max_open_conns"`
}

// Validate validates the database configuration.
func (cfg *DatabaseConfig) Validate() error {
	if cfg.Host == "" {
		return fmt.Errorf("malformed database host: ''")
	}
	// TODO:
	return nil
}

// GatewayConfig is the gateway server configuration.
type GatewayConfig struct {
	// HTTP is the gateway http endpoint config.
	HTTP *GatewayHTTPConfig `koanf:"http"`

	// Monitoring is the gateway prometheus configuration.
	Monitoring *GatewayMonitoringConfig `koanf:"monitoring"`

	// WS is the gateway websocket endpoint config.
	WS *GatewayWSConfig `koanf:"ws"`

	// ChainID defines the Ethereum network chain id.
	ChainID uint32 `koanf:"chain_id"`

	// MethodLimits is the gateway method limits config.
	MethodLimits *MethodLimits `koanf:"method_limits"`

	// OasisRPCs controls whether to enable the `oasis_*` methods. Default is not exposed.
	ExposeOasisRPCs bool `koanf:"oasis_rpcs"`
}

// GatewayMonitoringConfig is the gateway prometheus configuration.
type GatewayMonitoringConfig struct {
	// Host is the host interface on which to start the prometheus http server. Disabled if unset.
	Host string `koanf:"host"`

	// Port is the port number on which to start the prometheus http server.
	Port int `koanf:"port"`
}

// Enabled returns true if monitoring is configured.
func (cfg *GatewayMonitoringConfig) Enabled() bool {
	if cfg == nil {
		return false
	}
	if cfg.Host == "" {
		return false
	}
	return true
}

// Address returns the prometheus listen address.
//
// Returns empty string if monitoring is not configured.
func (cfg *GatewayMonitoringConfig) Address() string {
	if !cfg.Enabled() {
		return ""
	}
	return fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
}

// Validate validates the gateway configuration.
func (cfg *GatewayConfig) Validate() error {
	// TODO:
	return nil
}

type GatewayHTTPConfig struct {
	// Host is the host interface on which to start the HTTP RPC server. Defaults to localhost.
	Host string `koanf:"host"`

	// Port is the port number on which to start the HTTP RPC server. Defaults to 8545.
	Port int `koanf:"port"`

	// Cors are the CORS allowed urls.
	Cors []string `koanf:"cors"`

	// VirtualHosts is the list of virtual hostnames which are allowed on incoming requests.
	VirtualHosts []string `koanf:"virtual_hosts"`

	// PathPrefix specifies a path prefix on which http-rpc is to be served. Defaults to '/'.
	PathPrefix string `koanf:"path_prefix"`

	// Timeouts allows for customization of the timeout values used by the HTTP RPC
	// interface.
	Timeouts *HTTPTimeouts `koanf:"timeouts"`
}

type HTTPTimeouts struct {
	Read  *time.Duration `koanf:"read"`
	Write *time.Duration `koanf:"write"`
	Idle  *time.Duration `koanf:"idle"`
}

type GatewayWSConfig struct {
	// Host is the host interface on which to start the HTTP RPC server. Defaults to localhost.
	Host string `koanf:"host"`

	// Port is the port number on which to start the HTTP RPC server. Defaults to 8545.
	Port int `koanf:"port"`

	// PathPrefix specifies a path prefix on which http-rpc is to be served. Defaults to '/'.
	PathPrefix string `koanf:"path_prefix"`

	// Origins is the list of domain to accept websocket requests from.
	Origins []string `koanf:"origins"`

	// Timeouts allows for customization of the timeout values used by the HTTP RPC
	// interface.
	Timeouts *HTTPTimeouts `koanf:"timeouts"`
}

// MethodLimits are the configured gateway method limits.
type MethodLimits struct {
	// GetLogsMaxRounds is the maximum number of rounds to query for in a get logs query.
	GetLogsMaxRounds uint64 `koanf:"get_logs_max_rounds"`
}

// GasConfig is the gas price oracle configuration.
type GasConfig struct {
	// MinGasPrice is the minimum gas price to accept for transactions.
	MinGasPrice uint64 `koanf:"min_gas_price"`
	// BlockFullThreshold is the percentage block gas used threshold to consider a block full.
	BlockFullThreshold float64 `koanf:"block_full_threshold"`
	// WindowSize is the number of past blocks to consider for gas price computation.
	WindowSize uint64 `koanf:"window_size"`
	// ComputedPriceMargin is the gas price to add to the computed gas price.
	ComputedPriceMargin uint64 `koanf:"computed_price_margin"`
	// FeeHistorySize is the number of recent blocks to store for the fee history query.
	FeeHistorySize uint64 `koanf:"fee_history_size"`
}

// Validate validates the gas configuration.
func (cfg *GasConfig) Validate() error {
	if cfg.BlockFullThreshold < 0 || cfg.BlockFullThreshold > 1 {
		return fmt.Errorf("block full threshold must be in range [0, 1]")
	}
	return nil
}

// InitConfig initializes configuration from file.
func InitConfig(f string) (*Config, error) {
	var config Config
	k := koanf.New(".")

	// Load configuration from the yaml config.
	if err := k.Load(file.Provider(f), yaml.Parser()); err != nil {
		return nil, err
	}

	// Load environment variables and merge into the loaded config.
	if err := k.Load(env.Provider("", ".", func(s string) string {
		// `__` is used as a hierarchy delimiter.
		return strings.ReplaceAll(strings.ToLower(s), "__", ".")
	}), nil); err != nil {
		return nil, err
	}

	// Unmarshal into config.
	if err := k.Unmarshal("", &config); err != nil {
		return nil, err
	}

	// Validate config.
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &config, nil
}

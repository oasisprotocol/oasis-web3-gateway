package log

import (
	"fmt"
	"io"
	"os"

	"github.com/oasisprotocol/oasis-core/go/common/logging"

	"github.com/oasisprotocol/oasis-web3-gateway/conf"
)

// InitLogging initializes logging based on the provided configuration.
func InitLogging(cfg *conf.Config) error {
	var w io.Writer = os.Stdout
	format := logging.FmtLogfmt
	level := logging.LevelDebug
	if cfg.Log != nil {
		var err error
		if w, err = getLoggingStream(cfg.Log); err != nil {
			return fmt.Errorf("opening log file: %w", err)
		}
		if err := format.Set(cfg.Log.Format); err != nil {
			return err
		}
		if err := level.Set(cfg.Log.Level); err != nil {
			return err
		}
	}

	return logging.Initialize(w, format, level, nil)
}

func getLoggingStream(cfg *conf.LogConfig) (io.Writer, error) {
	if cfg == nil || cfg.File == "" {
		return os.Stdout, nil
	}
	w, err := os.OpenFile(cfg.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	return w, nil
}

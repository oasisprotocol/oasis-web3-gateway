package conf

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitConfig(t *testing.T) {
	cfg, err := InitConfig("tests.yml")
	require.NoError(t, err)

	fmt.Printf("host=%v, port=%v, db=%v\n", cfg.Database.Host, cfg.Database.Port, cfg.Database.DB)
	fmt.Printf("user=%v, password=%v\n", cfg.Database.User, cfg.Database.Password)
	fmt.Printf("timeout=%v\n", cfg.Database.Timeout)
}

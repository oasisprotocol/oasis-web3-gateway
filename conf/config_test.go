package conf

import (
	"fmt"
	"testing"
)

func TestInitConfig(t *testing.T) {
	cfg := InitConfig("server.yml")
	fmt.Printf("host=%v, port=%v, db=%v\n", cfg.Database.Host, cfg.Database.Port, cfg.Database.Db)
	fmt.Printf("user=%v, password=%v\n", cfg.Database.User, cfg.Database.Password)
	fmt.Printf("timeout=%v\n", cfg.Database.Timeout)
}

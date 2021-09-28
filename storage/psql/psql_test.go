package psql

import (
	"github.com/starfishlabs/oasis-evm-web3-gateway/conf"
	"log"
	"testing"
)

func TestInitPostDb(t *testing.T) {
	cfg, err := conf.InitConfig("../../conf/server.yml")
	if err != nil {
		log.Fatal("initialize config error:", err)
	}
	db, err := InitPostDb(cfg)
	if err != nil {
		log.Fatal("initialize postdb error:", err)
	}
	_ = db
}

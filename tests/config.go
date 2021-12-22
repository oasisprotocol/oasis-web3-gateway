package tests

import (
	"log"

	"github.com/oasisprotocol/oasis-evm-web3-gateway/conf"
)

var TestsConfig *conf.Config

// InitTestsConfig initializes configuration file for tests.
func InitTestsConfig() (err error) {
	TestsConfig, err = conf.InitConfig("../../conf/tests.yml")

	return
}

// MustInitConfig initializes configuration for tests or exits.
func MustInitConfig() {
	err := InitTestsConfig()
	if err != nil {
		log.Fatalf("failed to init config: %v", err)
	}
}

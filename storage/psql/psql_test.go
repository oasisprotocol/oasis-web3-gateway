package psql

import (
	"fmt"
	"github.com/starfishlabs/oasis-evm-web3-gateway/conf"
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
	"log"
	"testing"
)

func TestInitPostDb(t *testing.T) {
	cfg, err := conf.InitConfig("../../conf/server.yml")
	if err != nil {
		log.Fatal("initialize config error:", err)
	}
	db, err := InitDb(cfg)
	if err != nil {
		log.Fatal("initialize postdb error:", err)
	}
	block1 := &model.Block{
		Round: 1,
		Hash:  "hello",
	}
	block2 := &model.Block{
		Round: 2,
		Hash:  "world",
	}
	block3 := &model.Block{
		Round: 1,
		Hash:  "hello world",
	}
	db.Store(block1)
	db.Store(block2)
	db.Store(block3)
	round, err := db.GetBlockRound(block1.Hash)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("block1 round:", round)
	hash, err := db.GetBlockHash(block1.Round)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("block1 hash:", hash)

	tx1 := &model.Transaction{
		EthTx: "hello",
		Result: &model.TxResult{
			Hash:  "tx1 oasis hash",
			Index: 1,
			Round: 1,
		},
	}
	tx2 := &model.Transaction{
		EthTx: "hello",
		Result: &model.TxResult{
			Hash:  "tx2 oasis hash",
			Index: 1,
			Round: 2,
		},
	}
	db.Store(tx1)
	db.Store(tx2)
	res, err := db.GetTxResult(tx1.EthTx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("tx result: hash:%v, index:%v, round:%v\n", res.Hash, res.Index, res.Round)
}

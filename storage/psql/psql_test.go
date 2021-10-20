package psql

import (
	"fmt"
	"github.com/starfishlabs/oasis-evm-web3-gateway/conf"
	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
	"log"
	"math/big"
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
	block1 := &model.BlockRef{
		Round: 1,
		Hash:  "hello",
	}
	block2 := &model.BlockRef{
		Round: 2,
		Hash:  "world",
	}
	block3 := &model.BlockRef{
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

	tx1 := &model.TransactionRef{
		EthTxHash: "hello",
		Index:     1,
		Round:     1,
	}
	tx2 := &model.TransactionRef{
		EthTxHash: "hello",
		Index:     1,
		Round:     2,
	}
	db.Store(tx1)
	db.Store(tx2)
	round, index, err := db.GetTransactionRoundAndIndex(tx1.EthTxHash)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("index: %v, round: %v\n", index, round)

	legacyTx := &model.Transaction{
		Hash:       "hello",
		Type:       0,
		ChainID:    "0",
		Gas:        213144,
		GasPrice:   "123124",
		GasTipCap:  "0",
		GasFeeCap:  "0",
		Nonce:      1,
		To:         "hellohello",
		Value:      "4321000000000000000",
		Data:       "123456abcdef",
		AccessList: []model.AccessTuple{},
		V:          big.NewInt(1).String(),
		R:          big.NewInt(1).String(),
		S:          big.NewInt(1).String(),
	}
	accList := []model.AccessTuple{
		{Address: "helloworld", StorageKeys: []string{"hello", "world"}},
	}
	accessListTx := &model.Transaction{
		Hash:       "world",
		Type:       1,
		ChainID:    "12321",
		Gas:        43568,
		GasPrice:   "437231",
		GasTipCap:  "0",
		GasFeeCap:  "0",
		Nonce:      2,
		To:         "worldworld",
		Value:      "2137000000000000000",
		Data:       "abcdefabcdef",
		AccessList: accList,
		V:          big.NewInt(2).String(),
		R:          big.NewInt(2).String(),
		S:          big.NewInt(2).String(),
	}
	dynamicFeeTx := &model.Transaction{
		Hash:       "good",
		Type:       2,
		ChainID:    "45654",
		Gas:        2367215,
		GasPrice:   "0",
		GasTipCap:  "123123",
		GasFeeCap:  "345321",
		Nonce:      3,
		To:         "goodgood",
		Value:      "1123450000000000000",
		Data:       "123456123456",
		AccessList: accList,
		V:          big.NewInt(3).String(),
		R:          big.NewInt(3).String(),
		S:          big.NewInt(3).String(),
	}
	db.Store(legacyTx)
	db.Store(accessListTx)
	db.Store(dynamicFeeTx)

	tx, err := db.GetTransaction("hello")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("hash: %v, type: %v, chai_id: %v \n", tx.Hash, tx.Type, tx.ChainID)
	fmt.Printf("gas: %v, gas_price: %v, gas_fee_cap: %v, gas_tip_cap: %v\n", tx.Gas, tx.GasPrice, tx.GasTipCap, tx.GasTipCap)
	fmt.Printf("to: %v, value: %v\n", tx.To, tx.Value)
	fmt.Printf("access_list: %v\n", tx.AccessList)
}

func TestUpdate(t *testing.T) {
	cfg, err := conf.InitConfig("../../conf/server.yml")
	if err != nil {
		log.Fatal("initialize config error:", err)
	}
	db, err := InitDb(cfg)
	if err != nil {
		log.Fatal("initialize postdb error:", err)
	}

	ir1 := &model.ContinuesIndexedRound{
		Tip:   "tip",
		Round: 1,
	}
	if err := db.Update(ir1); err != nil {
		log.Fatalln("Update", err)
	}
	if r1, err := db.GetContinuesIndexedRound(); err != nil {
		log.Fatalln("GetContinuesIndexedRound", err)
	} else {
		fmt.Println("round:", r1)
	}

	ir2 := &model.ContinuesIndexedRound{
		Tip:   "tip",
		Round: 2,
	}
	if err := db.Update(ir2); err != nil {
		log.Fatalln("Update", err)
	}
	if r2, err := db.GetContinuesIndexedRound(); err != nil {
		log.Fatalln("GetContinuesIndexedRound", err)
	} else {
		fmt.Println("round:", r2)
	}

	ir3 := &model.ContinuesIndexedRound{
		Tip:   "tip",
		Round: 3,
	}
	if err := db.Update(ir3); err != nil {
		log.Fatalln("Update", err)
	}
	if r3, err := db.GetContinuesIndexedRound(); err != nil {
		log.Fatalln("GetContinuesIndexedRound", err)
	} else {
		fmt.Println("round:", r3)
	}
}

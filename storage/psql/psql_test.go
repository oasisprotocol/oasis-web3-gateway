package psql

import (
	"context"
	"log"
	"math/big"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"

	"github.com/oasisprotocol/oasis-web3-gateway/db/migrations"
	"github.com/oasisprotocol/oasis-web3-gateway/db/model"
	"github.com/oasisprotocol/oasis-web3-gateway/tests"
)

var db *PostDB

func TestMain(m *testing.M) {
	var err error
	ctx := context.Background()
	tests.MustInitConfig()
	db, err = InitDB(ctx, tests.TestsConfig.Database, false, false)
	if err != nil {
		log.Println(`It seems database failed to initialize. Do you have PostgreSQL running? If not, you can run
docker run  -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=postgres  -p 5432:5432 -d postgres`)
		log.Fatal("failed to initialize db:", err)
	}
	if err = db.RunMigrations(ctx); err != nil {
		log.Fatal("failed to run migrations:", err)
	}

	// Run tests.
	code := m.Run()

	if err = migrations.DropTables(ctx, db.DB.(*bun.DB)); err != nil {
		log.Fatal("failed to cleanup db:", err)
	}

	os.Exit(code)
}

func TestInitPostDb(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	block1 := &model.Block{
		Round:  1,
		Hash:   "hello",
		Header: &model.Header{},
	}
	block2 := &model.Block{
		Round:  2,
		Hash:   "world",
		Header: &model.Header{},
	}
	block3 := &model.Block{
		Round:  3,
		Hash:   "hello world",
		Header: &model.Header{},
	}
	if err := db.Upsert(ctx, block1); err != nil {
		log.Fatal("store error:", err)
	}
	if err := db.Upsert(ctx, block2); err != nil {
		log.Fatal("store error:", err)
	}
	if err := db.Upsert(ctx, block3); err != nil {
		log.Fatal("store error:", err)
	}
	round, err := db.GetBlockRound(ctx, block1.Hash)
	require.NoError(err)
	require.EqualValues(1, round, "GetBlockRound should return expected round")

	hash, err := db.GetBlockHash(ctx, block1.Round)
	require.NoError(err)
	require.EqualValues("hello", hash, "GetBlockHash should return expected hash")

	hash, err = db.GetLatestBlockHash(ctx)
	require.NoError(err)
	require.EqualValues("hello world", hash, "GetLatestBlockHash should return expected hash")

	legacyTx := &model.Transaction{
		Hash:       "hello",
		Type:       0,
		ChainID:    "0",
		Gas:        213144,
		GasPrice:   "123124",
		GasTipCap:  "0",
		GasFeeCap:  "0",
		Nonce:      1,
		ToAddr:     "hellohello",
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
		ToAddr:     "worldworld",
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
		ToAddr:     "goodgood",
		Value:      "1123450000000000000",
		Data:       "123456123456",
		AccessList: accList,
		V:          big.NewInt(3).String(),
		R:          big.NewInt(3).String(),
		S:          big.NewInt(3).String(),
	}
	err = db.Upsert(ctx, legacyTx)
	require.NoError(err, "unable to store legacy transaction")
	err = db.Upsert(ctx, accessListTx)
	require.NoError(err, "unable to store access list transaction")
	err = db.Upsert(ctx, dynamicFeeTx)
	require.NoError(err, "unable to store dynamic fee transaction")

	tx, err := db.GetTransaction(ctx, "hello")
	require.NoError(err)
	require.EqualValues(tx, legacyTx, "GetTransaction should return expected transaction")

	// Reinserting the same block should fail.
	if err = db.Insert(ctx, block3); err == nil {
		log.Fatal("Expected insert error on duplicate block, got: no error")
	}
}

func TestUpsert(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()
	ir1 := &model.IndexedRoundWithTip{
		Tip:   model.Continues,
		Round: 1,
	}
	require.NoError(db.Upsert(ctx, ir1), "update")

	r1, err := db.GetLastIndexedRound(ctx)
	require.NoError(err, "GetLastIndexedRound")
	require.EqualValues(1, r1)

	ir2 := &model.IndexedRoundWithTip{
		Tip:   model.Continues,
		Round: 2,
	}
	require.NoError(db.Upsert(ctx, ir2), "update")
	r2, err := db.GetLastIndexedRound(ctx)
	require.NoError(err, "GetLastIndexedRound")
	require.EqualValues(2, r2)

	ir3 := &model.IndexedRoundWithTip{
		Tip:   model.Continues,
		Round: 3,
	}
	require.NoError(db.Upsert(ctx, ir3), "update")
	r3, err := db.GetLastIndexedRound(ctx)
	require.NoError(err, "GetLastIndexedRound")
	require.EqualValues(3, r3)
}

func TestDelete(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	require.NoError(db.Delete(ctx, new(model.Block), 10), "delete")
}

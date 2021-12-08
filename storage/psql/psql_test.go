package psql

import (
	"log"
	"math/big"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/starfishlabs/oasis-evm-web3-gateway/model"
	"github.com/starfishlabs/oasis-evm-web3-gateway/tests"
)

var db *PostDB

func TestMain(m *testing.M) {
	var err error

	tests.MustInitConfig()
	db, err = InitDB(tests.TestsConfig.Database)
	if err != nil {
		log.Println(`It seems database failed to initialize. Do you have PostgreSQL running? If not, you can run
docker run  -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=postgres  -p 5432:5432 -d postgres`)
		log.Fatal("failed to initialize db:", err)
	}

	// Run tests.
	code := m.Run()

	if err = model.TruncateModel(db.DB); err != nil {
		log.Fatal("failed to cleanup db:", err)
	}

	os.Exit(code)
}

func TestInitPostDb(t *testing.T) {
	require := require.New(t)
	db, err := InitDB(tests.TestsConfig.Database)
	require.NoError(err, "initialize db")
	block1 := &model.BlockRef{
		Round: 1,
		Hash:  "hello",
	}
	block2 := &model.BlockRef{
		Round: 2,
		Hash:  "world",
	}
	block3 := &model.BlockRef{
		Round: 3,
		Hash:  "hello world",
	}
	if err = db.Store(block1); err != nil {
		log.Fatal("store error:", err)
	}
	if err = db.Store(block2); err != nil {
		log.Fatal("store error:", err)
	}
	if err = db.Store(block3); err != nil {
		log.Fatal("store error:", err)
	}
	round, err := db.GetBlockRound(block1.Hash)
	require.NoError(err)
	require.EqualValues(1, round, "GetBlockRound should return expected round")

	hash, err := db.GetBlockHash(block1.Round)
	require.NoError(err)
	require.EqualValues("hello", hash, "GetBlockHash should return expected hash")

	hash, err = db.GetLatestBlockHash()
	require.NoError(err)
	require.EqualValues("hello world", hash, "GetLatestBlockHash should return expected hash")

	tx1 := &model.TransactionRef{
		EthTxHash: "hello",
		Index:     1,
		Round:     1,
		BlockHash: "abc123",
	}
	tx2 := &model.TransactionRef{
		EthTxHash: "hello2",
		Index:     1,
		Round:     2,
		BlockHash: "cde456",
	}
	if err = db.Store(tx1); err != nil {
		log.Fatal("postdb store tx error:", err)
	}
	if err = db.Store(tx2); err != nil {
		log.Fatal("postdb store tx error:", err)
	}
	txRef, err := db.GetTransactionRef(tx1.EthTxHash)
	require.NoError(err)
	require.EqualValues(1, txRef.Index)
	require.EqualValues(1, txRef.Round)

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
	err = db.Store(legacyTx)
	require.NoError(err, "unable to store legacy transaction")
	err = db.Store(accessListTx)
	require.NoError(err, "unable to store access list transaction")
	err = db.Store(dynamicFeeTx)
	require.NoError(err, "unable to store dynamic fee transaction")

	tx, err := db.GetTransaction("hello")
	require.NoError(err)
	require.EqualValues(tx, legacyTx, "GetTransaction should return expected transaction")
}

func TestUpdate(t *testing.T) {
	require := require.New(t)

	db, err := InitDB(tests.TestsConfig.Database)
	require.NoError(err, "initialize db")
	ir1 := &model.IndexedRoundWithTip{
		Tip:   model.Continues,
		Round: 1,
	}
	require.NoError(db.Update(ir1), "update")

	r1, err := db.GetContinuesIndexedRound()
	require.NoError(err, "GetContinuesIndexedRound")
	require.EqualValues(1, r1)

	ir2 := &model.IndexedRoundWithTip{
		Tip:   model.Continues,
		Round: 2,
	}
	require.NoError(db.Update(ir2), "update")
	r2, err := db.GetContinuesIndexedRound()
	require.NoError(err, "GetContinuesIndexedRound")
	require.EqualValues(2, r2)

	ir3 := &model.IndexedRoundWithTip{
		Tip:   model.Continues,
		Round: 3,
	}
	require.NoError(db.Update(ir3), "update")
	r3, err := db.GetContinuesIndexedRound()
	require.NoError(err, "GetContinuesIndexedRound")
	require.EqualValues(3, r3)
}

func TestDelete(t *testing.T) {
	require := require.New(t)

	db, err := InitDB(tests.TestsConfig.Database)
	require.NoError(err, "initialize postdb")

	require.NoError(db.Delete(new(model.BlockRef), 10), "delete")
}

func TestGetBlockHash(t *testing.T) {
	require := require.New(t)

	_, err := InitDB(tests.TestsConfig.Database)
	require.NoError(err, "initialize db")

	// TODO: this fails as expected as the db doesn't contain the block.
	//       Forgot to initialize the db with the block?
	// hash, err := db.GetBlockHash(1)
	// require.NoError(err, "GetBlockHash")
	// fmt.Println("block hash:", hash)
}

func TestGetTransactionRef(t *testing.T) {
	require := require.New(t)

	_, err := InitDB(tests.TestsConfig.Database)
	require.NoError(err, "initialize db")

	// TODO: this fails as expected as the db doesn't contain the transaction.
	//       Forgot to initialize the db with the transaction?
	// txRef, err := db.GetTransactionRef("0xec826b483b27e3a4f9b68994d2f4768533ab4d1ae0b7d05867fcc9da18064715")
	// require.NoError(err, "GetTransactionRef")
	// fmt.Println(txRef.EthTxHash, txRef.BlockHash, txRef.Round, txRef.Index)
}

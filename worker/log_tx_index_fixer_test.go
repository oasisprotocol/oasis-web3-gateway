package worker

import (
	"context"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"

	"github.com/oasisprotocol/oasis-web3-gateway/conf"
	"github.com/oasisprotocol/oasis-web3-gateway/db/migrations"
	"github.com/oasisprotocol/oasis-web3-gateway/db/model"
	"github.com/oasisprotocol/oasis-web3-gateway/log"
	"github.com/oasisprotocol/oasis-web3-gateway/storage/psql"
)

const testTimeout = 5 * time.Second

func initTestDB(t *testing.T) (*bun.DB, *psql.PostDB) {
	// Find the config file relative to the project root.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	// Go up directories until we find conf/tests.yml
	configPath := ""
	for dir := wd; dir != "/" && dir != "."; dir = filepath.Dir(dir) {
		testPath := filepath.Join(dir, "conf", "tests.yml")
		if _, err := os.Stat(testPath); err == nil {
			configPath = testPath
			break
		}
	}

	if configPath == "" {
		t.Fatalf("could not find conf/tests.yml")
	}

	cfg, err := conf.InitConfig(configPath)
	if err != nil {
		t.Fatalf("failed to init config: %v", err)
	}

	// Initialize logging (ignore if already initialized).
	_ = log.InitLogging(cfg)

	ctx := context.Background()

	storage, err := psql.InitDB(ctx, cfg.Database, false, false)
	if err != nil {
		t.Fatalf("failed to initialize db: %v", err)
	}

	if err := storage.RunMigrations(ctx); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	db := storage.DB.(*bun.DB)
	return db, storage
}

func cleanupTestDB(t *testing.T, db *bun.DB) {
	if err := migrations.DropTables(context.Background(), db); err != nil {
		t.Logf("failed to cleanup db: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Logf("failed to close db: %v", err)
	}
}

func createTestData(ctx context.Context, t *testing.T, storage *psql.PostDB) (*model.Transaction, *model.Transaction, *model.Transaction) {
	require := require.New(t)

	tx1 := &model.Transaction{
		Hash:      "tx1",
		Type:      0,
		ChainID:   "1",
		BlockHash: "block1",
		Round:     100,
		Index:     0,
		Gas:       21000,
		GasPrice:  "1000000000",
		GasTipCap: "0",
		GasFeeCap: "0",
		Nonce:     1,
		FromAddr:  "0x1111111111111111111111111111111111111111",
		ToAddr:    "0x2222222222222222222222222222222222222222",
		Value:     "1000000000000000000",
		Data:      "",
		V:         big.NewInt(1).String(),
		R:         big.NewInt(1).String(),
		S:         big.NewInt(1).String(),
	}

	tx2 := &model.Transaction{
		Hash:      "tx2",
		Type:      0,
		ChainID:   "1",
		BlockHash: "block1",
		Round:     100,
		Index:     1,
		Gas:       21000,
		GasPrice:  "1000000000",
		GasTipCap: "0",
		GasFeeCap: "0",
		Nonce:     2,
		FromAddr:  "0x3333333333333333333333333333333333333333",
		ToAddr:    "0x4444444444444444444444444444444444444444",
		Value:     "2000000000000000000",
		Data:      "",
		V:         big.NewInt(1).String(),
		R:         big.NewInt(1).String(),
		S:         big.NewInt(1).String(),
	}

	tx3 := &model.Transaction{
		Hash:      "tx3",
		Type:      0,
		ChainID:   "1",
		BlockHash: "block2",
		Round:     200,
		Index:     0,
		Gas:       21000,
		GasPrice:  "1000000000",
		GasTipCap: "0",
		GasFeeCap: "0",
		Nonce:     3,
		FromAddr:  "0x5555555555555555555555555555555555555555",
		ToAddr:    "0x6666666666666666666666666666666666666666",
		Value:     "3000000000000000000",
		Data:      "",
		V:         big.NewInt(1).String(),
		R:         big.NewInt(1).String(),
		S:         big.NewInt(1).String(),
	}

	require.NoError(storage.Insert(ctx, tx1))
	require.NoError(storage.Insert(ctx, tx2))
	require.NoError(storage.Insert(ctx, tx3))

	log1 := &model.Log{
		Address:   "0x1111111111111111111111111111111111111111",
		Topics:    []string{"0x1111111111111111111111111111111111111111111111111111111111111111"},
		Data:      "0x1234",
		Round:     100,
		BlockHash: "block1",
		TxHash:    tx1.Hash,
		TxIndex:   0, // Correct.
		Index:     0,
		Removed:   false,
	}

	log2 := &model.Log{
		Address:   "0x2222222222222222222222222222222222222222",
		Topics:    []string{"0x2222222222222222222222222222222222222222222222222222222222222222"},
		Data:      "0x5678",
		Round:     100,
		BlockHash: "block1",
		TxHash:    tx2.Hash,
		TxIndex:   2, // INCORRECT! Should be 1.
		Index:     1,
		Removed:   false,
	}

	log3 := &model.Log{
		Address:   "0x3333333333333333333333333333333333333333",
		Topics:    []string{"0x3333333333333333333333333333333333333333333333333333333333333333"},
		Data:      "0x9abc",
		Round:     200,
		BlockHash: "block2",
		TxHash:    tx3.Hash,
		TxIndex:   0, // Correct.
		Index:     0,
		Removed:   false,
	}

	require.NoError(storage.Insert(ctx, log1))
	require.NoError(storage.Insert(ctx, log2))
	require.NoError(storage.Insert(ctx, log3))

	return tx1, tx2, tx3
}

func TestLogTxIndexFixer_EmptyDatabase(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	db, _ := initTestDB(t)
	defer cleanupTestDB(t, db)

	worker := NewLogTxIndexFixer(db)

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := worker.Start(ctx); err != nil {
			t.Errorf("worker failed: %v", err)
		}
	}()

	select {
	case <-done:
		// Worker completed (should be quick for empty database).
	case <-time.After(testTimeout):
		t.Fatal("worker did not complete within timeout for empty database")
	}
}

func TestLogTxIndexFixer_FixesMismatches(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	db, storage := initTestDB(t)
	defer cleanupTestDB(t, db)

	tx1, tx2, tx3 := createTestData(ctx, t, storage)

	// Verify initial incorrect state.
	var log2TxIndex uint
	err := db.NewSelect().
		Model((*model.Log)(nil)).
		Column("tx_index").
		Where("tx_hash = ?", tx2.Hash).
		Scan(ctx, &log2TxIndex)
	require.NoError(err)
	require.Equal(uint(2), log2TxIndex)

	worker := NewLogTxIndexFixer(db)

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := worker.Start(ctx); err != nil {
			t.Errorf("worker failed: %v", err)
		}
	}()

	select {
	case <-done:
		// Worker completed.
	case <-time.After(testTimeout):
		t.Fatal("worker did not complete within timeout")
	}

	// Verify log2 was corrected.
	err = db.NewSelect().
		Model((*model.Log)(nil)).
		Column("tx_index").
		Where("tx_hash = ?", tx2.Hash).
		Scan(ctx, &log2TxIndex)
	require.NoError(err)
	require.Equal(uint(1), log2TxIndex)

	// Verify correct logs were not changed.
	var log1TxIndex uint
	err = db.NewSelect().
		Model((*model.Log)(nil)).
		Column("tx_index").
		Where("tx_hash = ?", tx1.Hash).
		Scan(ctx, &log1TxIndex)
	require.NoError(err)
	require.Equal(uint(0), log1TxIndex)

	var log3TxIndex uint
	err = db.NewSelect().
		Model((*model.Log)(nil)).
		Column("tx_index").
		Where("tx_hash = ?", tx3.Hash).
		Scan(ctx, &log3TxIndex)
	require.NoError(err)
	require.Equal(uint(0), log3TxIndex)
}

package psql

import (
	"context"
	"fmt"
	"reflect"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/uptrace/bun"

	"github.com/oasisprotocol/oasis-web3-gateway/db/model"
	"github.com/oasisprotocol/oasis-web3-gateway/storage"
)

var durations = promauto.NewHistogramVec(prometheus.HistogramOpts{Name: "oasis_oasis_web3_gateway_psql_query_seconds", Buckets: []float64{0.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}, Help: "Histogram for the postgresql query duration."}, []string{"query"})

func measureDuration(label string) func() {
	timer := prometheus.NewTimer(durations.WithLabelValues(label))
	return func() {
		timer.ObserveDuration()
	}
}

func getTypeName(t interface{}) string {
	return reflect.TypeOf(t).String()
}

type metricsWrapper struct {
	s storage.Storage
}

// Delete implements storage.Storage.
func (m *metricsWrapper) Delete(ctx context.Context, table interface{}, round uint64) error {
	defer measureDuration(fmt.Sprintf("Delete(%s)", getTypeName(table)))()

	return m.s.Delete(ctx, table, round)
}

// GetBlockByHash implements storage.Storage.
func (m *metricsWrapper) GetBlockByHash(ctx context.Context, hash string) (*model.Block, error) {
	defer measureDuration("GetBlockByHash")()

	return m.s.GetBlockByHash(ctx, hash)
}

// GetBlockByNumber implements storage.Storage.
func (m *metricsWrapper) GetBlockByNumber(ctx context.Context, round uint64) (*model.Block, error) {
	defer measureDuration("GetBlockByNumber")()

	return m.s.GetBlockByNumber(ctx, round)
}

// GetBlockHash implements storage.Storage.
func (m *metricsWrapper) GetBlockHash(ctx context.Context, round uint64) (string, error) {
	defer measureDuration("GetBlockHash")()

	return m.s.GetBlockHash(ctx, round)
}

// GetBlockRound implements storage.Storage.
func (m *metricsWrapper) GetBlockRound(ctx context.Context, hash string) (uint64, error) {
	defer measureDuration("GetBlockRound")()

	return m.s.GetBlockRound(ctx, hash)
}

// GetBlockTransaction implements storage.Storage.
func (m *metricsWrapper) GetBlockTransaction(ctx context.Context, blockHash string, txIndex int) (*model.Transaction, error) {
	defer measureDuration("GetBlockTransaction")()

	return m.s.GetBlockTransaction(ctx, blockHash, txIndex)
}

// GetBlockTransactionCountByHash implements storage.Storage.
func (m *metricsWrapper) GetBlockTransactionCountByHash(ctx context.Context, hash string) (int, error) {
	defer measureDuration("GetBlockTransactionCountByHash")()

	return m.s.GetBlockTransactionCountByHash(ctx, hash)
}

// GetBlockTransactionCountByNumber implements storage.Storage.
func (m *metricsWrapper) GetBlockTransactionCountByNumber(ctx context.Context, round uint64) (int, error) {
	defer measureDuration("GetBlockTransactionCountByNumber")()

	return m.s.GetBlockTransactionCountByNumber(ctx, round)
}

// GetLastIndexedRound implements storage.Storage.
func (m *metricsWrapper) GetLastIndexedRound(ctx context.Context) (uint64, error) {
	defer measureDuration("GetLastIndexedRound")()

	return m.s.GetLastIndexedRound(ctx)
}

// GetLastRetainedRound implements storage.Storage.
func (m *metricsWrapper) GetLastRetainedRound(ctx context.Context) (uint64, error) {
	defer measureDuration("GetLastRetainedRound")()

	return m.s.GetLastRetainedRound(ctx)
}

// GetLatestBlockHash implements storage.Storage.
func (m *metricsWrapper) GetLatestBlockHash(ctx context.Context) (string, error) {
	defer measureDuration("GetLastestBlockHash")()

	return m.s.GetLatestBlockHash(ctx)
}

// GetLatestBlockNumber implements storage.Storage.
func (m *metricsWrapper) GetLatestBlockNumber(ctx context.Context) (uint64, error) {
	defer measureDuration("GetLastestBlockNumber")()

	return m.s.GetLatestBlockNumber(ctx)
}

// GetLogs implements storage.Storage.
func (m *metricsWrapper) GetLogs(ctx context.Context, startRound uint64, endRound uint64) ([]*model.Log, error) {
	defer measureDuration("GetLogs")()

	return m.s.GetLogs(ctx, startRound, endRound)
}

// GetTransaction implements storage.Storage.
func (m *metricsWrapper) GetTransaction(ctx context.Context, hash string) (*model.Transaction, error) {
	defer measureDuration("GetTransaction")()

	return m.s.GetTransaction(ctx, hash)
}

// GetTransactionReceipt implements storage.Storage.
func (m *metricsWrapper) GetTransactionReceipt(ctx context.Context, txHash string) (*model.Receipt, error) {
	defer measureDuration("GetTransactionReceipt")()

	return m.s.GetTransactionReceipt(ctx, txHash)
}

// Insert implements storage.Storage.
func (m *metricsWrapper) Insert(ctx context.Context, value interface{}) error {
	defer measureDuration(fmt.Sprintf("Insert(%s)", getTypeName(value)))()

	return m.s.Insert(ctx, value)
}

// InsertIfNotExists implements storage.Storage.
func (m *metricsWrapper) InsertIfNotExists(ctx context.Context, value interface{}) error {
	defer measureDuration(fmt.Sprintf("InsertIfNotExists(%s)", getTypeName(value)))()

	return m.s.InsertIfNotExists(ctx, value)
}

// RunInTransaction implements storage.Storage.
func (m *metricsWrapper) RunInTransaction(ctx context.Context, fn func(storage.Storage) error) error {
	postDB, ok := m.s.(*PostDB)
	if !ok {
		return fmt.Errorf("unsupported backend")
	}

	bdb, ok := postDB.DB.(*bun.DB)
	if !ok {
		return fmt.Errorf("already in a transaction")
	}

	return bdb.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		db := NewMetricsWrapper(transactionStorage(&tx))
		return fn(db)
	})
}

// Upsert implements storage.Storage.
func (m *metricsWrapper) Upsert(ctx context.Context, value interface{}) error {
	defer measureDuration(fmt.Sprintf("Upsert(%s)", getTypeName(value)))()

	return m.s.Upsert(ctx, value)
}

// NewMetricsWrapper returns an instrumanted storage.
func NewMetricsWrapper(s storage.Storage) storage.Storage {
	return &metricsWrapper{
		s,
	}
}

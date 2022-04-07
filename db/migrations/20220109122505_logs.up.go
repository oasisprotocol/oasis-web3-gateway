package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/oasisprotocol/oasis-core/go/common/logging"
	"github.com/uptrace/bun"
)

const batchSize = 100

// Storage is a db storage helper wrapper.
//
// Avoid depending on storage gateway types and methods as the migration should
// be immutable and work on the state of the previous migration. Whereas the
// existing storage type and methods will always be compatible with the most
// recent db version.
type Storage struct {
	DB bun.IDB
}

type IndexedRoundWithTip struct {
	Tip   string `bun:",pk"`
	Round uint64
}

// Continues is the latest Indexed Block Round.
const Continues string = "continues"

// LastRetained is the block with minimum height maintained.
const LastRetained string = "lastRetain"

// Log  is the log representation in db.
type Log struct {
	Address   string
	Topics    []string
	Data      string
	Round     uint64 // BlockNumber
	BlockHash string
	TxHash    string `bun:",pk"`
	TxIndex   uint
	Index     uint `bun:",pk"`
	Removed   bool
}

// Transaction is transaction representation in db.
type Transaction struct {
	Hash      string `bun:",pk"`
	Type      uint8
	Status    uint // tx/receipt status
	ChainID   string
	BlockHash string
	Round     uint64
	Index     uint32
	Gas       uint64
	GasPrice  string
	GasTipCap string
	GasFeeCap string
	Nonce     uint64
	FromAddr  string
	ToAddr    string
	Value     string
	Data      string
	V, R, S   string
}

// GetLastRetainedRound returns the minimum round not pruned.
func (s *Storage) GetLastRetainedRound(ctx context.Context) (uint64, error) {
	retainedRound := new(IndexedRoundWithTip)
	err := s.DB.NewSelect().Model(retainedRound).Where("tip = ?", LastRetained).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}

	return retainedRound.Round, nil
}

// GetLastIndexedRound returns latest indexed block round.
func (s *Storage) GetLastIndexedRound(ctx context.Context) (uint64, error) {
	indexedRound := new(IndexedRoundWithTip)
	err := s.DB.NewSelect().Model(indexedRound).Where("tip = ?", Continues).Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}

	return indexedRound.Round, nil
}

// GetLogs return the logs for the round.
func (s *Storage) GetLogs(ctx context.Context, startRound, endRound uint64) ([]*Log, error) {
	logs := []*Log{}
	err := s.DB.NewSelect().Model(&logs).
		Where("round BETWEEN ? AND ?", startRound, endRound).
		Order("round ASC").
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	return logs, nil
}

// DeleteLogs deletes logs between rounds.
func (s *Storage) DeleteLogs(ctx context.Context, startRound uint64, endRound uint64) error {
	logs := []*Log{}
	_, err := s.DB.NewDelete().Model(&logs).
		Where("round BETWEEN ? AND ?", startRound, endRound).
		Exec(ctx)
	return err
}

// InsertLogs inserts a batch of logs into the db.
func (s *Storage) InsertLogs(ctx context.Context, values []*Log) error {
	_, err := s.DB.NewInsert().Model(&values).Exec(ctx)
	return err
}

// Upsert upserts a value.
func (s *Storage) Upsert(ctx context.Context, value interface{}) error {
	typ := reflect.TypeOf(value)
	table := s.DB.Dialect().Tables().Get(typ)
	pks := make([]string, len(table.PKs))
	for i, f := range table.PKs {
		pks[i] = f.Name
	}

	_, err := s.DB.NewInsert().
		Model(value).
		On(fmt.Sprintf("CONFLICT (%s) DO UPDATE", strings.Join(pks, ","))).
		Exec(ctx)

	return err
}

// LogsUp does performes the 20220109122505_logs up migration.
func LogsUp(ctx context.Context, tx *bun.Tx) error {
	logger := logging.GetLogger("migration")

	storage := Storage{DB: tx}

	// Get first round.
	start, err := storage.GetLastRetainedRound(ctx)
	if err != nil {
		return fmt.Errorf("GetLastRetainedRound: %w", err)
	}
	// Get last round.
	end, err := storage.GetLastIndexedRound(ctx)
	if err != nil {
		return fmt.Errorf("GetLastIndexedRound: %w", err)
	}

	logger.Info("starting migration", "start_round", start, "end_round", end)

	for i := start; i <= end; i += batchSize {
		bs := i
		be := i + batchSize - 1
		if be > end {
			be = end
		}

		logger.Debug("migrating batch", "batch_start", bs, "batch_end", be)
		// Fetch all logs for the batch rounds.
		var logs []*Log
		logs, err = storage.GetLogs(ctx, bs, be)
		if err != nil {
			return fmt.Errorf("GetLogs: %w", err)
		}

		// Since index is part of the PK, we need to delete the old records instead of upserting.
		if err = storage.DeleteLogs(ctx, bs, be); err != nil {
			return fmt.Errorf("DeleteLogs: %w", err)
		}

		currentIdx := uint(0)
		currentRound := bs
		for _, l := range logs {
			if l.Round != currentRound {
				// Reset index.
				currentIdx = 0
				currentRound = l.Round
			}
			l.Index = currentIdx
			currentIdx++
		}

		if len(logs) > 0 {
			// Insert the new records.
			if err = storage.InsertLogs(ctx, logs); err != nil {
				return fmt.Errorf("BulkInsert: %w", err)
			}
		}

		logger.Debug("migrated batch", "batch_start", bs, "batch_end", be, "logs", len(logs))
	}

	logger.Info("migration complete")

	return nil
}

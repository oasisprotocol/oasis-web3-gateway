// Package worker provides background workers for maintenance tasks.
package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"github.com/oasisprotocol/oasis-core/go/common/logging"
)

const (
	batchSize        = 10_000
	workerSleepTime  = 1 * time.Second
	backoffSleepTime = 30 * time.Second
)

// LogTxIndexFixer is a background worker that fixes log tx_index mismatches.
//
// Fixes historical log tx_index data that was incorrectly indexed prior to
// the fix in issue #786 (https://github.com/oasisprotocol/oasis-web3-gateway/issues/786).
type LogTxIndexFixer struct {
	db     *bun.DB
	logger *logging.Logger
}

// NewLogTxIndexFixer creates a new LogTxIndexFixer worker.
func NewLogTxIndexFixer(db *bun.DB) *LogTxIndexFixer {
	return &LogTxIndexFixer{
		db:     db,
		logger: logging.GetLogger("log-tx-index-fixer"),
	}
}

// Start starts the worker.
func (w *LogTxIndexFixer) Start(ctx context.Context) error {
	w.logger.Info("starting log tx_index fixer worker")

	// Find the highest mismatched round once at the start.
	highestMismatchedRound, err := w.findHighestMismatchedRound(ctx)
	if err != nil {
		return fmt.Errorf("failed to find highest mismatched round: %w", err)
	}

	if highestMismatchedRound == 0 {
		w.logger.Info("all log tx_index mismatches fixed, worker stopping")
		return nil
	}

	w.logger.Info("found mismatches, starting fix process", "highest_round", highestMismatchedRound)

	// Process batches from highest round down to 0.
	currentEndRound := highestMismatchedRound
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("log tx_index fixer worker stopped")
			return ctx.Err()
		default:
		}

		// Calculate batch range.
		var startRound uint64
		if currentEndRound >= batchSize {
			startRound = currentEndRound - batchSize + 1
		} else {
			startRound = 0
		}

		w.logger.Debug("fixing batch", "start_round", startRound, "end_round", currentEndRound)

		rowsAffected, err := w.fixRoundRange(ctx, startRound, currentEndRound)
		if err != nil {
			w.logger.Error("error fixing log tx_index batch", "err", err, "start_round", startRound, "end_round", currentEndRound)
			time.Sleep(backoffSleepTime)
			continue
		}

		w.logger.Debug("fixed log tx_index batch",
			"start_round", startRound,
			"end_round", currentEndRound,
			"rows_affected", rowsAffected)

		// Move to next batch, unless done.
		if startRound == 0 {
			break
		}
		currentEndRound = startRound - 1

		time.Sleep(workerSleepTime)
	}

	w.logger.Info("all log tx_index mismatches fixed, worker stopping")
	return nil
}

// findHighestMismatchedRound finds the highest round number that has log tx_index mismatches.
func (w *LogTxIndexFixer) findHighestMismatchedRound(ctx context.Context) (uint64, error) {
	query := `
		SELECT l.round
		FROM logs l
		JOIN transactions t ON l.tx_hash = t.hash
		WHERE l.tx_index != t.index
		ORDER BY l.round DESC
		LIMIT 1
	`

	var round *uint64
	err := w.db.NewRaw(query).Scan(ctx, &round)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil // No mismatches found
		}
		return 0, fmt.Errorf("finding highest mismatched round: %w", err)
	}

	if round == nil {
		return 0, nil // No mismatches found
	}

	return *round, nil
}

// fixRoundRange fixes log tx_index mismatches for a range of rounds.
func (w *LogTxIndexFixer) fixRoundRange(ctx context.Context, startRound, endRound uint64) (int64, error) {
	query := `
		UPDATE logs
		SET tx_index = t.index
		FROM transactions t
		WHERE logs.tx_hash = t.hash
		AND logs.round BETWEEN ? AND ?
		AND logs.tx_index != t.index
	`

	result, err := w.db.NewRaw(query, startRound, endRound).Exec(ctx)
	if err != nil {
		return 0, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	return rowsAffected, nil
}

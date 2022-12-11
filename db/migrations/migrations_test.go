package migrations

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"

	"github.com/oasisprotocol/oasis-web3-gateway/db/migrator"
	"github.com/oasisprotocol/oasis-web3-gateway/tests"
)

func TestConcurentMigrations(t *testing.T) {
	ctx := context.Background()
	tests.MustInitConfig()
	psql := InitTests(t)
	defer func() {
		_ = DropTables(ctx, psql)
	}()

	// Add a dummy migration that ensures it is called only once.
	var migrationRuns uint32
	Migrations.Add(migrator.Migration{
		Name: "20500109122505",
		Up: func(ctx context.Context, db *bun.Tx) error {
			new := atomic.AddUint32(&migrationRuns, 1)
			require.EqualValues(t, 1, new, "migration run more than once")
			return nil
		},
	})

	require.NoError(t, Init(ctx, psql), "migrator init")

	var wg sync.WaitGroup
	// Run multiple migrations concurrently.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			require.NoError(t, Migrate(ctx, psql), "failed to run migrations")
		}()
	}

	wg.Wait()
}

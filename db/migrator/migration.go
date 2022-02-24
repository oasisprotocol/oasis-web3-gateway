package migrator

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/uptrace/bun"
)

type Migration struct {
	bun.BaseModel

	ID         int64 `bun:",pk,autoincrement"`
	Name       string
	MigratedAt time.Time `bun:",notnull,nullzero,default:current_timestamp"`

	Up   MigrationFunc `bun:"-"`
	Down MigrationFunc `bun:"-"`

	// Unused, just here so that this is backwards compatible.
	GroupID int64
}

func (m *Migration) String() string {
	return m.Name
}

func (m *Migration) IsApplied() bool {
	return m.ID > 0
}

type MigrationFunc func(ctx context.Context, db *bun.Tx) error

func NewSQLMigrationFunc(fsys fs.FS, name string) MigrationFunc {
	return func(ctx context.Context, tx *bun.Tx) error {
		f, err := fsys.Open(name)
		if err != nil {
			return err
		}

		scanner := bufio.NewScanner(f)
		var queries []string

		var query []byte
		for scanner.Scan() {
			b := scanner.Bytes()

			const prefix = "--bun:"
			if bytes.HasPrefix(b, []byte(prefix)) {
				b = b[len(prefix):]
				if bytes.Equal(b, []byte("split")) {
					queries = append(queries, string(query))
					query = query[:0]
					continue
				}
				return fmt.Errorf("bun: unknown directive: %q", b)
			}

			query = append(query, b...)
			query = append(query, '\n')
		}

		if len(query) > 0 {
			queries = append(queries, string(query))
		}
		if err = scanner.Err(); err != nil {
			return err
		}

		for _, q := range queries {
			_, err = tx.ExecContext(ctx, q)
			if err != nil {
				return err
			}
		}

		return nil
	}
}

type MigrationSlice []Migration

func (ms MigrationSlice) String() string {
	if len(ms) == 0 {
		return "empty"
	}

	if len(ms) > 5 {
		return fmt.Sprintf("%d migrations (%s ... %s)", len(ms), ms[0].Name, ms[len(ms)-1].Name)
	}

	var sb strings.Builder

	for i := range ms {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(ms[i].Name)
	}

	return sb.String()
}

// Applied returns applied migrations in descending order
// (the order is important and is used in Rollback).
func (ms MigrationSlice) Applied() MigrationSlice {
	var applied MigrationSlice
	for i := range ms {
		if ms[i].IsApplied() {
			applied = append(applied, ms[i])
		}
	}
	sortDesc(applied)
	return applied
}

// Unapplied returns unapplied migrations in ascending order
// (the order is important and is used in Migrate).
func (ms MigrationSlice) Unapplied() MigrationSlice {
	var unapplied MigrationSlice
	for i := range ms {
		if !ms[i].IsApplied() {
			unapplied = append(unapplied, ms[i])
		}
	}
	sortAsc(unapplied)
	return unapplied
}

type MigrationFile struct {
	Name    string
	Path    string
	Content string
}

func sortAsc(ms MigrationSlice) {
	sort.Slice(ms, func(i, j int) bool {
		return ms[i].Name < ms[j].Name
	})
}

func sortDesc(ms MigrationSlice) {
	sort.Slice(ms, func(i, j int) bool {
		return ms[i].Name > ms[j].Name
	})
}

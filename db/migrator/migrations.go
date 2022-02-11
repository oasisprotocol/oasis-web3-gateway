package migrator

type Migrations struct {
	ms MigrationSlice
}

func NewMigrations() *Migrations {
	m := new(Migrations)
	return m
}

func (m *Migrations) Sorted() MigrationSlice {
	migrations := make(MigrationSlice, len(m.ms))
	copy(migrations, m.ms)
	sortAsc(migrations)
	return migrations
}

func (m *Migrations) Add(migration Migration) {
	if migration.Name == "" {
		panic("migration name is required")
	}
	m.ms = append(m.ms, migration)
}

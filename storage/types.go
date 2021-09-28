package storage

import "database/sql"

type PostDb struct {
	Db *sql.DB
}

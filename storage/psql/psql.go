package psql

import (
	"database/sql"
	"errors"
	"fmt"
	_ "github.com/lib/pq"
	"github.com/starfishlabs/oasis-evm-web3-gateway/conf"
	"github.com/starfishlabs/oasis-evm-web3-gateway/storage"
)

// InitPostDb create postdb instance
func InitPostDb(cfg *conf.Config) (*storage.PostDb, error) {
	if cfg == nil {
		return nil, errors.New("nil configuration")
	}
	conn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=%d",
		cfg.PostDb.Host, cfg.PostDb.Port, cfg.PostDb.User, cfg.PostDb.Password, cfg.PostDb.Db, cfg.PostDb.SslMode, cfg.PostDb.Timeout)

	db, err := sql.Open("postgres", conn)
	if err != nil {
		return nil, err
	}
	if err = db.Ping(); err != nil {
		return nil, err
	}
	return &storage.PostDb{
		Db: db,
	}, nil
}

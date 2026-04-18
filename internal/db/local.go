package db

import (
	"database/sql"
	"fmt"

	"github.com/phuc-nt/dandori-cli/internal/config"
	_ "modernc.org/sqlite"
)

type LocalDB struct {
	db *sql.DB
}

func Open(path string) (*LocalDB, error) {
	if path == "" {
		var err error
		path, err = config.DBPath()
		if err != nil {
			return nil, err
		}
	}

	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return &LocalDB{db: db}, nil
}

func (l *LocalDB) Close() error {
	return l.db.Close()
}

func (l *LocalDB) DB() *sql.DB {
	return l.db
}

func (l *LocalDB) Exec(query string, args ...any) (sql.Result, error) {
	return l.db.Exec(query, args...)
}

func (l *LocalDB) Query(query string, args ...any) (*sql.Rows, error) {
	return l.db.Query(query, args...)
}

func (l *LocalDB) QueryRow(query string, args ...any) *sql.Row {
	return l.db.QueryRow(query, args...)
}

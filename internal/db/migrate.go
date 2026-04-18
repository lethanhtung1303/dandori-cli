package db

import (
	"fmt"
)

func (l *LocalDB) Migrate() error {
	currentVersion, err := l.getSchemaVersion()
	if err != nil {
		return fmt.Errorf("get schema version: %w", err)
	}

	if currentVersion >= SchemaVersion {
		return nil
	}

	if currentVersion == 0 {
		if _, err := l.Exec(SchemaSQL); err != nil {
			return fmt.Errorf("apply schema: %w", err)
		}
		if err := l.setSchemaVersion(SchemaVersion); err != nil {
			return fmt.Errorf("set schema version: %w", err)
		}
	}

	return nil
}

func (l *LocalDB) getSchemaVersion() (int, error) {
	var tableExists int
	err := l.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='schema_version'
	`).Scan(&tableExists)
	if err != nil {
		return 0, err
	}

	if tableExists == 0 {
		return 0, nil
	}

	var version int
	err = l.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&version)
	if err != nil {
		return 0, err
	}

	return version, nil
}

func (l *LocalDB) setSchemaVersion(version int) error {
	_, err := l.Exec(`INSERT INTO schema_version (version) VALUES (?)`, version)
	return err
}

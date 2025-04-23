package scheduler

import (
	"database/sql"
	"encoding/json"
	"log"
	"time"

	"github.com/feedloop/qhronos/internal/config"
	"github.com/jmoiron/sqlx"
)

const (
	archiveLockKey = 12345 // Arbitrary unique key for advisory lock
)

func acquireArchiveLock(db *sqlx.DB) (bool, error) {
	var gotLock bool
	err := db.Get(&gotLock, "SELECT pg_try_advisory_lock($1)", archiveLockKey)
	return gotLock, err
}

func releaseArchiveLock(db *sqlx.DB) error {
	_, err := db.Exec("SELECT pg_advisory_unlock($1)", archiveLockKey)
	return err
}

func shouldArchive(db *sqlx.DB, checkPeriod time.Duration) (bool, error) {
	var lastTime time.Time
	err := db.Get(&lastTime, "SELECT value::timestamptz FROM system_config WHERE key = 'last_archival_time'")
	if err != nil {
		if err == sql.ErrNoRows {
			return true, nil // Never archived before
		}
		return false, err
	}
	if time.Since(lastTime) < checkPeriod {
		return false, nil
	}
	return true, nil
}

func updateLastArchivalTime(db *sqlx.DB) error {
	_, err := db.Exec(`
		INSERT INTO system_config (key, value, description, updated_by)
		VALUES ('last_archival_time', now(), 'Last archival run', 'system')
		ON CONFLICT (key) DO UPDATE SET value = now(), updated_at = now(), updated_by = 'system'
	`)
	return err
}

func syncRetentionConfigToDB(db *sqlx.DB, retention config.RetentionConfig) error {
	value := map[string]interface{}{
		"events": map[string]interface{}{
			"max_past_occurrences": retention.Events.String(),
		},
		"occurrences": map[string]interface{}{
			"max_past_occurrences": retention.Occurrences.String(),
		},
	}
	valueJSON, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		INSERT INTO system_config (key, value, description, updated_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now(), updated_by = EXCLUDED.updated_by
	`, "retention_policies", valueJSON, "Data retention policies", "system")
	return err
}

func StartArchivalScheduler(db *sqlx.DB, checkPeriod time.Duration, retention config.RetentionConfig, stopCh <-chan struct{}) {
	// Sync retention config on startup
	if err := syncRetentionConfigToDB(db, retention); err != nil {
		log.Printf("[archiver] Failed to sync retention config: %v", err)
	}
	ticker := time.NewTicker(checkPeriod)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				gotLock, err := acquireArchiveLock(db)
				if err != nil {
					log.Printf("[archiver] Error acquiring lock: %v", err)
					continue
				}
				if !gotLock {
					log.Printf("[archiver] Another instance is archiving, skipping this cycle.")
					continue
				}
				shouldRun, err := shouldArchive(db, checkPeriod)
				if err != nil {
					log.Printf("[archiver] Error checking last archival time: %v", err)
					releaseArchiveLock(db)
					continue
				}
				if !shouldRun {
					log.Printf("[archiver] Archival already run within the period, skipping.")
					releaseArchiveLock(db)
					continue
				}
				_, err = db.Exec("SELECT archive_old_data($1)", retention.Events.String())
				if err != nil {
					log.Printf("[archiver] Error running archive_old_data: %v", err)
					releaseArchiveLock(db)
					continue
				}
				if err := updateLastArchivalTime(db); err != nil {
					log.Printf("[archiver] Error updating last archival time: %v", err)
				}
				log.Printf("[archiver] Archival completed successfully.")
				releaseArchiveLock(db)
			case <-stopCh:
				log.Printf("[archiver] Stopping archival scheduler.")
				return
			}
		}
	}()
}

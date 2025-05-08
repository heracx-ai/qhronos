package scheduler

import (
	"context"
	"time"

	"github.com/feedloop/qhronos/internal/config"
	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const (
	archiveLockKey      = 12345 // Arbitrary unique key for advisory lock
	lastArchivalTimeKey = "archiver:last_archival_time"
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

func shouldArchiveRedis(ctx context.Context, rdb *redis.Client, checkPeriod time.Duration) (bool, error) {
	val, err := rdb.Get(ctx, lastArchivalTimeKey).Result()
	if err == redis.Nil {
		// Not set yet, set to now + checkPeriod
		nextTime := time.Now().Add(checkPeriod)
		err := rdb.Set(ctx, lastArchivalTimeKey, nextTime.Format(time.RFC3339), 0).Err()
		if err != nil {
			return false, err
		}
		return true, nil // Should archive now
	} else if err != nil {
		return false, err
	}
	lastTime, err := time.Parse(time.RFC3339, val)
	if err != nil {
		return false, err
	}
	if time.Now().Before(lastTime) {
		return false, nil
	}
	return true, nil
}

func updateLastArchivalTimeRedis(ctx context.Context, rdb *redis.Client, checkPeriod time.Duration) error {
	nextTime := time.Now().Add(checkPeriod)
	return rdb.Set(ctx, lastArchivalTimeKey, nextTime.Format(time.RFC3339), 0).Err()
}

func syncRetentionConfigToDB(db *sqlx.DB, durations config.RetentionDurations) error {
	/*value := map[string]interface{}{
		"events": map[string]interface{}{
			"max_past_occurrences": durations.Events.String(),
		},
		"occurrences": map[string]interface{}{
			"max_past_occurrences": durations.Occurrences.String(),
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
	return err*/
	return nil // Prevent writing to system_config
}

func StartArchivalScheduler(db *sqlx.DB, rdb *redis.Client, checkPeriod time.Duration, durations config.RetentionDurations, stopCh <-chan struct{}, logger *zap.Logger) {
	// Sync retention config on startup
	if err := syncRetentionConfigToDB(db, durations); err != nil {
		logger.Error("[archiver] Failed to sync retention config", zap.Error(err))
	}
	ticker := time.NewTicker(checkPeriod)
	ctx := context.Background()
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				gotLock, err := acquireArchiveLock(db)
				if err != nil {
					logger.Error("[archiver] Error acquiring lock", zap.Error(err))
					continue
				}
				if !gotLock {
					logger.Debug("[archiver] Another instance is archiving, skipping this cycle.")
					continue
				}
				shouldRun, err := shouldArchiveRedis(ctx, rdb, checkPeriod)
				if err != nil {
					logger.Error("[archiver] Error checking last archival time", zap.Error(err))
					releaseArchiveLock(db)
					continue
				}
				if !shouldRun {
					logger.Debug("[archiver] Archival already run within the period, skipping.")
					releaseArchiveLock(db)
					continue
				}
				_, err = db.Exec("SELECT archive_old_data($1)", durations.Events.String())
				if err != nil {
					logger.Error("[archiver] Error running archive_old_data", zap.Error(err))
					releaseArchiveLock(db)
					continue
				}
				if err := updateLastArchivalTimeRedis(ctx, rdb, checkPeriod); err != nil {
					logger.Error("[archiver] Error updating last archival time", zap.Error(err))
				}
				logger.Info("[archiver] Archival completed successfully.")
				releaseArchiveLock(db)
			case <-stopCh:
				logger.Info("[archiver] Stopping archival scheduler.")
				return
			}
		}
	}()
}

package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/rsb/garmin-weight-sync/internal/domain"
)

// MySQLRepository implements domain.MeasurementRepository using MySQL.
type MySQLRepository struct {
	db *sql.DB
}

// NewMySQLRepository creates a new MySQLRepository instance and runs migrations.
func NewMySQLRepository(db *sql.DB) (*MySQLRepository, error) {
	repo := &MySQLRepository{db: db}
	if err := repo.migrate(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}
	return repo, nil
}

// migrate creates the measurements table if it doesn't exist, or, if an older
// one-row-per-entry table is found, consolidates it into one row per day.
func (r *MySQLRepository) migrate() error {
	log.Println("Running database migrations...")

	exists, err := r.tableExists("measurements")
	if err != nil {
		return fmt.Errorf("failed to check for measurements table: %w", err)
	}
	if !exists {
		return r.createTable()
	}

	hasDateCol, err := r.columnExists("measurements", "measurement_date")
	if err != nil {
		return fmt.Errorf("failed to inspect measurements table: %w", err)
	}
	if hasDateCol {
		return nil
	}

	return r.consolidateToOneRowPerDay()
}

func (r *MySQLRepository) createTable() error {
	query := `
	CREATE TABLE IF NOT EXISTS measurements (
		measurement_date DATE PRIMARY KEY,
		apple_health_id VARCHAR(255) NULL,
		bmi DOUBLE NULL,
		fat_percentage DOUBLE NULL,
		lean_body_mass DOUBLE NULL,
		weight DOUBLE NULL,
		timestamp BIGINT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`
	_, err := r.db.Exec(query)
	return err
}

func (r *MySQLRepository) tableExists(table string) (bool, error) {
	var count int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?;`,
		table,
	).Scan(&count)
	return count > 0, err
}

func (r *MySQLRepository) columnExists(table, column string) (bool, error) {
	var count int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?;`,
		table, column,
	).Scan(&count)
	return count > 0, err
}

// consolidateToOneRowPerDay migrates the legacy one-row-per-entry table into the
// new one-row-per-day schema. For each day, the latest non-null value of each
// metric wins. The legacy table is kept around as measurements_legacy for backup.
func (r *MySQLRepository) consolidateToOneRowPerDay() error {
	log.Println("Legacy measurements schema detected; consolidating to one row per day...")

	if _, err := r.db.Exec(`RENAME TABLE measurements TO measurements_legacy;`); err != nil {
		return fmt.Errorf("failed to rename legacy table: %w", err)
	}

	if err := r.createTable(); err != nil {
		return fmt.Errorf("failed to create new measurements table: %w", err)
	}

	// Widen GROUP_CONCAT so late-in-day rows aren't silently truncated out of the pick.
	if _, err := r.db.Exec(`SET SESSION group_concat_max_len = 1000000;`); err != nil {
		return fmt.Errorf("failed to set group_concat_max_len: %w", err)
	}

	insert := `
	INSERT INTO measurements (measurement_date, apple_health_id, bmi, fat_percentage, lean_body_mass, weight, timestamp)
	SELECT
		DATE(FROM_UNIXTIME(timestamp / 1000)) AS measurement_date,
		SUBSTRING_INDEX(GROUP_CONCAT(apple_health_id ORDER BY timestamp DESC), ',', 1) AS apple_health_id,
		SUBSTRING_INDEX(GROUP_CONCAT(bmi ORDER BY timestamp DESC), ',', 1) AS bmi,
		SUBSTRING_INDEX(GROUP_CONCAT(fat_percentage ORDER BY timestamp DESC), ',', 1) AS fat_percentage,
		SUBSTRING_INDEX(GROUP_CONCAT(lean_body_mass ORDER BY timestamp DESC), ',', 1) AS lean_body_mass,
		SUBSTRING_INDEX(GROUP_CONCAT(weight ORDER BY timestamp DESC), ',', 1) AS weight,
		MAX(timestamp) AS timestamp
	FROM measurements_legacy
	GROUP BY DATE(FROM_UNIXTIME(timestamp / 1000));`

	if _, err := r.db.Exec(insert); err != nil {
		return fmt.Errorf("failed to consolidate legacy rows: %w", err)
	}

	log.Println("Consolidation complete. Legacy rows preserved in measurements_legacy; drop it manually once verified.")
	return nil
}

// Save inserts or merges a body composition measurement into that day's row.
func (r *MySQLRepository) Save(ctx context.Context, m *domain.BodyComposition) error {
	query := `
	INSERT INTO measurements (measurement_date, apple_health_id, bmi, fat_percentage, lean_body_mass, weight, timestamp)
	VALUES (DATE(FROM_UNIXTIME(? / 1000)), ?, ?, ?, ?, ?, ?)
	ON DUPLICATE KEY UPDATE
		apple_health_id = VALUES(apple_health_id),
		bmi = IF(VALUES(bmi) IS NOT NULL, VALUES(bmi), bmi),
		fat_percentage = IF(VALUES(fat_percentage) IS NOT NULL, VALUES(fat_percentage), fat_percentage),
		lean_body_mass = IF(VALUES(lean_body_mass) IS NOT NULL, VALUES(lean_body_mass), lean_body_mass),
		weight = IF(VALUES(weight) IS NOT NULL, VALUES(weight), weight),
		timestamp = VALUES(timestamp);`

	_, err := r.db.ExecContext(
		ctx,
		query,
		m.Timestamp,
		m.AppleHealthID,
		toNullFloat64(m.BMI),
		toNullFloat64(m.FatPercentage),
		toNullFloat64(m.LeanBodyMass),
		toNullFloat64(m.Weight),
		m.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("database query failed: %w", err)
	}
	return nil
}

// toNullFloat64 converts a float64 to sql.NullFloat64. If value is <= 0, it is treated as NULL.
func toNullFloat64(val float64) sql.NullFloat64 {
	return sql.NullFloat64{
		Float64: val,
		Valid:   val > 0,
	}
}

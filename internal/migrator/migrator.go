package migrator

import (
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrator manages database migrations.
type Migrator struct {
	db     *sqlx.DB
	log    *slog.Logger
	schema string
}

// NewMigrator creates a new migrator instance.
func NewMigrator(db *sqlx.DB, log *slog.Logger, schema string) *Migrator {
	return &Migrator{
		db:     db,
		log:    log,
		schema: schema,
	}
}

// Run executes all pending migrations.
func (m *Migrator) Run() error {
	op := "migrator.Run"
	m.log.Info("starting database migrations")

	if err := m.createMigrationsTable(); err != nil {
		return fmt.Errorf("%s: failed to create migrations table: %w", op, err)
	}

	migrations, err := m.getMigrationFiles()
	if err != nil {
		return fmt.Errorf("%s: failed to get migration files: %w", op, err)
	}

	for _, migration := range migrations {
		if err := m.runMigration(migration); err != nil {
			return fmt.Errorf("%s: failed to run migration %s: %w", op, migration, err)
		}
	}

	m.log.Info("database migrations completed successfully")
	return nil
}

func (m *Migrator) createMigrationsTable() error {
	schemaQuery := fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, m.schema)
	if _, err := m.db.Exec(schemaQuery); err != nil {
		return err
	}

	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s.schema_migrations (
			version VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`, m.schema)
	_, err := m.db.Exec(query)
	return err
}

func (m *Migrator) getMigrationFiles() ([]string, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}

	var migrations []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			migrations = append(migrations, entry.Name())
		}
	}

	sort.Strings(migrations)
	return migrations, nil
}

func (m *Migrator) isMigrationApplied(version string) (bool, error) {
	var count int
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s.schema_migrations WHERE version = $1`, m.schema)
	err := m.db.Get(&count, query, version)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (m *Migrator) runMigration(filename string) error {
	version := strings.TrimSuffix(filename, ".sql")

	applied, err := m.isMigrationApplied(version)
	if err != nil {
		return err
	}

	if applied {
		m.log.Debug("migration already applied", slog.String("version", version))
		return nil
	}

	m.log.Info("applying migration", slog.String("version", version))

	content, err := migrationsFS.ReadFile("migrations/" + filename)
	if err != nil {
		return fmt.Errorf("failed to read migration file: %w", err)
	}

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Set search_path for this transaction
	if _, err = tx.Exec(fmt.Sprintf("SET search_path TO %s, public", m.schema)); err != nil {
		return fmt.Errorf("failed to set search_path: %w", err)
	}

	if _, err = tx.Exec(string(content)); err != nil {
		return fmt.Errorf("failed to execute migration: %w", err)
	}

	insertQuery := fmt.Sprintf(
		`INSERT INTO %s.schema_migrations (version) VALUES ($1)`, m.schema)
	if _, err = tx.Exec(insertQuery, version); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	m.log.Info("migration applied successfully", slog.String("version", version))
	return nil
}

// GetAppliedMigrations returns the list of applied migrations.
func (m *Migrator) GetAppliedMigrations() ([]string, error) {
	var versions []string
	query := fmt.Sprintf(
		`SELECT version FROM %s.schema_migrations ORDER BY applied_at DESC`, m.schema)
	err := m.db.Select(&versions, query)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	return versions, nil
}

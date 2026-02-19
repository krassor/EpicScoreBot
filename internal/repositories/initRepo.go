package repositories

import (
	"EpicScoreBot/internal/config"
	"EpicScoreBot/internal/migrator"
	"EpicScoreBot/internal/utils/logger/sl"
	"context"
	"fmt"
	"log/slog"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// Repository provides access to the database.
type Repository struct {
	DB     *sqlx.DB
	log    *slog.Logger
	schema string
}

// New creates a new repository, connects to the database, and runs migrations.
func New(logger *slog.Logger, cfg *config.Config) *Repository {
	op := "repositories.New()"
	log := logger.With(
		slog.String("op", op))

	username := cfg.DBConfig.User
	password := cfg.DBConfig.Password
	dbName := cfg.DBConfig.Name
	dbHost := cfg.DBConfig.Host
	dbPort := cfg.DBConfig.Port
	schema := cfg.DBConfig.Schema

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s dbname=%s sslmode=disable password=%s search_path=%s",
		dbHost, dbPort, username, dbName, password, schema)

	conn, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		log.Error("error connecting to database", sl.Err(err))
		panic("error connecting to database")
	}

	if err := conn.Ping(); err != nil {
		log.Error("error pinging database", sl.Err(err))
		panic("error pinging database")
	}

	log.Debug("sqlx connected to database")

	m := migrator.NewMigrator(conn, log, schema)
	if err := m.Run(); err != nil {
		log.Error("error running database migrations", sl.Err(err))
		panic("error running database migrations")
	}

	return &Repository{
		DB:     conn,
		log:    log,
		schema: schema,
	}
}

// Shutdown closes the database connection.
func (r *Repository) Shutdown(ctx context.Context) error {
	op := "Repository.Shutdown"
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("force exit %s: %w", op, ctx.Err())
		default:
			if err := r.DB.Close(); err != nil {
				return fmt.Errorf("error exit %s: %w", op, err)
			}
			return nil
		}
	}
}

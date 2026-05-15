// Package main runs database migrations using golang-migrate.
// Usage:
//
//	migrate                  — apply all pending up migrations
//	migrate -down            — roll back the last migration
//	migrate -down -steps N   — roll back N migrations
//	migrate -version         — print the current schema version
//	migrate -path ./db/migrations   — override the default migrations directory
package main

import (
	"cashpilotx/internal/config"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	var (
		down    = flag.Bool("down", false, "roll back migrations instead of applying them")
		steps   = flag.Int("steps", 1, "number of steps when rolling back (only used with -down)")
		version = flag.Bool("version", false, "print the current migration version and exit")
		path    = flag.String("path", "db/migrations", "path to the migrations directory")
	)
	flag.Parse()

	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"}).
		With().Timestamp().Logger()

	cfg, err := config.LoadConfig(".")
	if err != nil {
		log.Error().Err(err).Msg("failed to load config")
		os.Exit(1)
	}
	if cfg.DBSource == "" {
		log.Error().Msg("DATABASE_URL is not set")
		os.Exit(1)
	}

	if err := run(cfg.DBSource, *path, *version, *down, *steps); err != nil {
		log.Error().Err(err).Msg("migration error")
		os.Exit(1)
	}
}

func run(dbSource, migrationsPath string, printVersion, down bool, steps int) error {
	sourceURL := fmt.Sprintf("file://%s", migrationsPath)
	m, err := migrate.New(sourceURL, dbSource)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer closeMigrator(m)

	if printVersion {
		return showVersion(m)
	}
	if down {
		return rollback(m, steps)
	}
	return applyUp(m)
}

func showVersion(m *migrate.Migrate) error {
	v, dirty, err := m.Version()
	if err != nil {
		if errors.Is(err, migrate.ErrNilVersion) {
			log.Info().Msg("no migrations applied yet (version: nil)")
			return nil
		}
		return fmt.Errorf("read version: %w", err)
	}
	log.Info().Uint("version", v).Bool("dirty", dirty).Msg("current migration version")
	return nil
}

func rollback(m *migrate.Migrate, steps int) error {
	log.Info().Int("steps", steps).Msg("rolling back migrations")
	if err := m.Steps(-steps); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			log.Info().Msg("no migrations to roll back")
			return nil
		}
		return fmt.Errorf("rollback: %w", err)
	}
	log.Info().Msg("rollback complete")
	return nil
}

func applyUp(m *migrate.Migrate) error {
	log.Info().Msg("applying pending migrations")
	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			log.Info().Msg("schema is already up to date")
			return nil
		}
		return fmt.Errorf("up: %w", err)
	}
	log.Info().Msg("migrations applied successfully")
	return nil
}

func closeMigrator(m *migrate.Migrate) {
	srcErr, dbErr := m.Close()
	if srcErr != nil {
		log.Warn().Err(srcErr).Msg("error closing migration source")
	}
	if dbErr != nil {
		log.Warn().Err(dbErr).Msg("error closing migration database connection")
	}
}

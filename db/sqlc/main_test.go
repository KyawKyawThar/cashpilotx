package db

import (
	"cashpilotx/internal/config"
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

var testDB *pgxpool.Pool
var testStore Store

func TestMain(m *testing.M) {
	// Database integration tests require a running PostgreSQL instance.
	// Skip the entire package when running in short mode (e.g. pre-push hook).
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}

	cfg, err := config.LoadConfig("../../")
	if err != nil {
		log.Fatal("cannot load config:", err)
	}

	testDB, err = pgxpool.New(context.Background(), cfg.DBSource)
	if err != nil {
		log.Fatal(err)
	}

	migration, err := migrate.New(
		"file://../../db/migrations",
		cfg.DBSource,
	)
	if err != nil {
		log.Fatal("cannot create migrate instance:", err)
	}
	if err = migration.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatal("failed to run migrate up:", err)
	}

	log.Println("db migrated successfully")

	testStore = NewStore(testDB)

	code := m.Run()
	testDB.Close()
	os.Exit(code)
}

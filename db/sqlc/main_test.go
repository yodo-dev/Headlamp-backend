package db

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/The-You-School-HeadLamp/headlamp_backend/util"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

var testStore Store

func TestMain(m *testing.M) {
	config, err := util.LoadConfig("../..")
	if err != nil {
		log.Fatal("cannot load config:", err)
	}

	connPool, err := pgxpool.New(context.Background(), config.DBSource)
	if err != nil {
		log.Fatal("cannot connect to db:", err)
	}

	testStore = NewStore(connPool)

	// run migrations
	migration, err := migrate.New(
		"file://../migration",
		config.DBSource,
	)
	if err != nil {
		log.Fatal("cannot create new migrate instance:", err)
	}

	if err = migration.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatal("failed to run migrate up:", err)
	}

	code := m.Run()

	// run migrations down
	if err = migration.Down(); err != nil && err != migrate.ErrNoChange {
		log.Fatal("failed to run migrate down:", err)
	}

	os.Exit(code)
}

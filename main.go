package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"net/http"

	"github.com/The-You-School-HeadLamp/headlamp_backend/api"
	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/gpt"
	"github.com/The-You-School-HeadLamp/headlamp_backend/token"
	"github.com/The-You-School-HeadLamp/headlamp_backend/util"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var interruptSignals = []os.Signal{
	os.Interrupt,
	syscall.SIGTERM,
	syscall.SIGINT,
}

func main() {

	config, err := util.LoadConfig(".")
	if err != nil {
		log.Fatal().Err(err).Msg("cannot load config")
	}

	if config.Environment == "DEVELOPMENT" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	ctx, stop := signal.NotifyContext(context.Background(), interruptSignals...)
	defer stop()

	connPool, err := pgxpool.New(ctx, config.DBSource)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot connect to db")
	}

	runDBMigration(config.MigrationURL, config.DBSource)

	store := db.NewStore(connPool)

	tokenMaker, err := token.NewPasetoMaker(config.TokenSymmetricKey)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot create token maker")
	}

	gptClient := gpt.NewGptClient(config.OpenAIAPIKey)

	runGinServer(ctx, config, store, tokenMaker, gptClient)

}

func runGinServer(ctx context.Context, config util.Config, store db.Store, tokenMaker token.Maker, gptClient gpt.GptClient) {
	server, err := api.NewServer(config, store, tokenMaker, gptClient)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot create server")
	}

	go func() {
		if err := server.Start(config.HTTPServerAddress); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("cannot start server")
		}
	}()

	<-ctx.Done()
	log.Info().Msg("shutting down server...")

	// Stop the reflection scheduler before closing the HTTP server
	server.StopScheduler()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatal().Err(err).Msg("server shutdown failed")
	}

	log.Info().Msg("server exited properly")
}

func runDBMigration(migrationURL string, dbSource string) {
	migration, err := migrate.New(migrationURL, dbSource)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot create new migrate instance")
	}

	if err = migration.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatal().Err(err).Msg("failed to run migrate up")
	}

	log.Info().Msg("db migrated successfully")
}

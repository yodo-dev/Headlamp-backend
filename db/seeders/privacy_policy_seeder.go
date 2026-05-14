package main

import (
	"context"
	"flag"
	"os"
	"strings"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/util"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const defaultDocumentKey = "privacy_policy"

func main() {
	filePath := flag.String("file", "privacy-policy/privacy-policy-content.txt", "Path to privacy policy content file")
	title := flag.String("title", "Privacy Policy", "Document title")
	version := flag.String("version", "v1", "Document version")
	documentKey := flag.String("key", defaultDocumentKey, "Document key")
	flag.Parse()

	config, err := util.LoadConfig(".")
	if err != nil {
		log.Fatal().Err(err).Msg("cannot load config")
	}

	if strings.EqualFold(config.Environment, "DEVELOPMENT") {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	content, err := os.ReadFile(*filePath)
	if err != nil {
		log.Fatal().Err(err).Str("file", *filePath).Msg("cannot read privacy policy file")
	}

	ctx := context.Background()
	connPool, err := pgxpool.New(ctx, config.DBSource)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot connect to db")
	}
	defer connPool.Close()

	store := db.NewStore(connPool)
	doc, err := store.UpsertPrivacyPolicyDocument(ctx, db.UpsertPrivacyPolicyDocumentParams{
		DocumentKey: strings.TrimSpace(*documentKey),
		Title:       strings.TrimSpace(*title),
		Version:     strings.TrimSpace(*version),
		Content:     string(content),
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to seed privacy policy")
	}

	log.Info().
		Int64("id", doc.ID).
		Str("document_key", doc.DocumentKey).
		Str("version", doc.Version).
		Time("updated_at", doc.UpdatedAt).
		Msg("privacy policy seeded successfully")
}

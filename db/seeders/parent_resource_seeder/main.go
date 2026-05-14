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

const defaultParentResourceDocumentKey = "parent_critical_thinking"

func main() {
	filePath := flag.String("file", "headlamp-parent-resource/HL-Parent-Resource-Critical-Thinking-Article.txt", "Path to parent resource content file")
	title := flag.String("title", "Parent- Critical Thinking", "Document title")
	version := flag.String("version", "v1", "Document version")
	documentKey := flag.String("key", defaultParentResourceDocumentKey, "Document key")
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
		log.Fatal().Err(err).Str("file", *filePath).Msg("cannot read parent resource file")
	}

	normalizedContent := normalizeParentResourceContent(string(content))

	ctx := context.Background()
	connPool, err := pgxpool.New(ctx, config.DBSource)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot connect to db")
	}
	defer connPool.Close()

	store := db.NewStore(connPool)
	doc, err := store.UpsertParentResourceDocument(ctx, db.UpsertParentResourceDocumentParams{
		DocumentKey: strings.TrimSpace(*documentKey),
		Title:       strings.TrimSpace(*title),
		Version:     strings.TrimSpace(*version),
		Content:     normalizedContent,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("failed to seed parent resource")
	}

	log.Info().
		Int64("id", doc.ID).
		Str("document_key", doc.DocumentKey).
		Str("version", doc.Version).
		Time("updated_at", doc.UpdatedAt).
		Msg("parent resource seeded successfully")
}

func normalizeParentResourceContent(content string) string {
	trimmed := strings.ReplaceAll(content, "\r\n", "\n")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return ""
	}

	lines := strings.Split(trimmed, "\n")
	if len(lines) > 1 {
		firstLine := strings.TrimSpace(lines[0])
		if strings.Contains(strings.ToLower(firstLine), "parent resource") {
			trimmed = strings.TrimSpace(strings.Join(lines[1:], "\n"))
		}
	}

	return trimmed
}

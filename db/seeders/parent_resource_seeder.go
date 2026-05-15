package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/util"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const defaultParentResourceDocumentKey = "parent_critical_thinking"
const defaultParentResourceFilePath = "content-topics/headlamp-parent-resource/HL-Parent-Resource-Critical-Thinking-Article.txt"

func runParentResourceSeeder(filePath, title, version, documentKey string) error {
	if strings.TrimSpace(filePath) == "" {
		filePath = defaultParentResourceFilePath
	}

	config, err := util.LoadConfig(".")
	if err != nil {
		return err
	}

	if strings.EqualFold(config.Environment, "DEVELOPMENT") {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read parent resource file %s: %w", filePath, err)
	}

	normalizedContent := normalizeParentResourceContent(string(content))

	ctx := context.Background()
	connPool, err := pgxpool.New(ctx, config.DBSource)
	if err != nil {
		return err
	}
	defer connPool.Close()

	store := db.NewStore(connPool)
	doc, err := store.UpsertParentResourceDocument(ctx, db.UpsertParentResourceDocumentParams{
		DocumentKey: strings.TrimSpace(documentKey),
		Title:       strings.TrimSpace(title),
		Version:     strings.TrimSpace(version),
		Content:     normalizedContent,
	})
	if err != nil {
		return err
	}

	log.Info().
		Int64("id", doc.ID).
		Str("document_key", doc.DocumentKey).
		Str("version", doc.Version).
		Time("updated_at", doc.UpdatedAt).
		Msg("parent resource seeded successfully")

	return nil
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

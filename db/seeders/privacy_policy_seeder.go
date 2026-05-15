package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/util"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const defaultDocumentKey = "privacy_policy"
const defaultPrivacyPolicyFilePath = "content-topics/privacy-policy/privacy-policy-content.txt"

func main() {
	seeder := flag.String("seeder", "privacy-policy", "Seeder to run: privacy-policy | parent-resource | content-topics")
	filePath := flag.String("file", defaultPrivacyPolicyFilePath, "Path to content file (used by privacy-policy and parent-resource seeders)")
	title := flag.String("title", "Privacy Policy", "Document title")
	version := flag.String("version", "v1", "Document version")
	documentKey := flag.String("key", defaultDocumentKey, "Document key")
	flag.Parse()

	target := strings.ToLower(strings.TrimSpace(*seeder))
	selectedFile := strings.TrimSpace(*filePath)
	selectedTitle := strings.TrimSpace(*title)
	selectedVersion := strings.TrimSpace(*version)
	selectedKey := strings.TrimSpace(*documentKey)

	if target == "parent-resource" {
		if selectedFile == "" || selectedFile == defaultPrivacyPolicyFilePath {
			selectedFile = defaultParentResourceFilePath
		}
		if selectedTitle == "" || selectedTitle == "Privacy Policy" {
			selectedTitle = "Parent- Critical Thinking"
		}
		if selectedKey == "" || selectedKey == defaultDocumentKey {
			selectedKey = defaultParentResourceDocumentKey
		}
	}

	var err error
	switch target {
	case "privacy-policy":
		err = runPrivacyPolicySeeder(selectedFile, selectedTitle, selectedVersion, selectedKey)
	case "parent-resource":
		err = runParentResourceSeeder(selectedFile, selectedTitle, selectedVersion, selectedKey)
	case "content-topics":
		err = runContentTopicsSeeder()
	default:
		err = fmt.Errorf("unsupported seeder %q", target)
	}

	if err != nil {
		log.Fatal().Err(err).Str("seeder", target).Msg("seeder failed")
	}
}

func runPrivacyPolicySeeder(filePath, title, version, documentKey string) error {
	if strings.TrimSpace(filePath) == "" {
		filePath = defaultPrivacyPolicyFilePath
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
		return fmt.Errorf("read privacy policy file %s: %w", filePath, err)
	}

	ctx := context.Background()
	connPool, err := pgxpool.New(ctx, config.DBSource)
	if err != nil {
		return err
	}
	defer connPool.Close()

	store := db.NewStore(connPool)
	doc, err := store.UpsertPrivacyPolicyDocument(ctx, db.UpsertPrivacyPolicyDocumentParams{
		DocumentKey: strings.TrimSpace(documentKey),
		Title:       strings.TrimSpace(title),
		Version:     strings.TrimSpace(version),
		Content:     string(content),
	})
	if err != nil {
		return err
	}

	log.Info().
		Int64("id", doc.ID).
		Str("document_key", doc.DocumentKey).
		Str("version", doc.Version).
		Time("updated_at", doc.UpdatedAt).
		Msg("privacy policy seeded successfully")

	return nil
}

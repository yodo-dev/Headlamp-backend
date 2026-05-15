package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/util"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type topicSeed struct {
	Category  string
	TopicKey  string
	Title     string
	Subtitle  string
	Version   string
	SortOrder int32
	FilePath  string
}

func resolveConversationFilePath(baseDir, marker string) (string, error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return "", err
	}

	needle := strings.ToLower(strings.TrimSpace(marker))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if strings.Contains(name, needle) {
			return filepath.Join(baseDir, entry.Name()), nil
		}
	}

	return "", os.ErrNotExist
}

func runContentTopicsSeeder() error {
	config, err := util.LoadConfig(".")
	if err != nil {
		return err
	}

	if strings.EqualFold(config.Environment, "DEVELOPMENT") {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	ctx := context.Background()
	connPool, err := pgxpool.New(ctx, config.DBSource)
	if err != nil {
		return err
	}
	defer connPool.Close()

	store := db.NewStore(connPool)

	legacyConversationKeys := []string{
		"how-to-talk-about-the-first-phone",
		"when-they-ask-for-social-media",
	}
	for _, legacyKey := range legacyConversationKeys {
		if _, err := connPool.Exec(ctx,
			"DELETE FROM content_topic_documents WHERE category = $1 AND topic_key = $2",
			"conversations",
			legacyKey,
		); err != nil {
			return err
		}
	}

	conversationsDir := filepath.Clean("content-topics/conversations")
	realFriendsFile, err := resolveConversationFilePath(conversationsDir, "real friends")
	if err != nil {
		return err
	}
	digitalRhythmFile, err := resolveConversationFilePath(conversationsDir, "digital rhythm")
	if err != nil {
		return err
	}

	items := []topicSeed{
		{
			Category:  "conversations",
			TopicKey:  "hl-parent-conversation-real-friends-real-life",
			Title:     "HL Parent Conversation- Real Friends, Real Life",
			Subtitle:  "Conversation Script - Phase 1",
			Version:   "v1",
			SortOrder: 1,
			FilePath:  realFriendsFile,
		},
		{
			Category:  "conversations",
			TopicKey:  "hl-parent-conversation-your-familys-digital-rhythm",
			Title:     "HL Parent Conversation- Your Family's Digital Rhythm",
			Subtitle:  "Conversation Script - Phase 2",
			Version:   "v1",
			SortOrder: 2,
			FilePath:  digitalRhythmFile,
		},
	}

	for _, item := range items {
		contentPath := filepath.Clean(item.FilePath)
		content, err := os.ReadFile(contentPath)
		if err != nil {
			return err
		}

		doc, err := store.UpsertContentTopicDocument(ctx, db.UpsertContentTopicDocumentParams{
			Category:  item.Category,
			TopicKey:  item.TopicKey,
			Title:     item.Title,
			Subtitle:  item.Subtitle,
			Version:   item.Version,
			SortOrder: item.SortOrder,
			Content:   string(content),
		})
		if err != nil {
			return err
		}

		log.Info().
			Int64("id", doc.ID).
			Str("category", doc.Category).
			Str("topic_key", doc.TopicKey).
			Str("version", doc.Version).
			Time("updated_at", doc.UpdatedAt).
			Msg("content topic seeded successfully")
	}

	return nil
}

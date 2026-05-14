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

func main() {
	config, err := util.LoadConfig(".")
	if err != nil {
		log.Fatal().Err(err).Msg("cannot load config")
	}

	if strings.EqualFold(config.Environment, "DEVELOPMENT") {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	ctx := context.Background()
	connPool, err := pgxpool.New(ctx, config.DBSource)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot connect to db")
	}
	defer connPool.Close()

	store := db.NewStore(connPool)

	items := []topicSeed{
		{
			Category:  "conversations",
			TopicKey:  "how-to-talk-about-the-first-phone",
			Title:     "How to talk about the first phone",
			Subtitle:  "Conversation Script - Phase 1",
			Version:   "v1",
			SortOrder: 1,
			FilePath:  "content-topics/conversations_how_to_talk_about_the_first_phone.txt",
		},
		{
			Category:  "conversations",
			TopicKey:  "when-they-ask-for-social-media",
			Title:     "When they ask for social media",
			Subtitle:  "Conversation Script - Phase 2",
			Version:   "v1",
			SortOrder: 2,
			FilePath:  "content-topics/conversations_when_they_ask_for_social_media.txt",
		},
		{
			Category:  "guides",
			TopicKey:  "setting-limits-without-starting-a-war",
			Title:     "Setting Limits Without Starting a War",
			Subtitle:  "Guide - 8min",
			Version:   "v1",
			SortOrder: 1,
			FilePath:  "content-topics/guides_setting_limits_without_starting_a_war.txt",
		},
		{
			Category:  "guides",
			TopicKey:  "what-dopamine-does-to-a-teens-brain",
			Title:     "What Dopamine Does to a Teen's Brain",
			Subtitle:  "Guide - 6min",
			Version:   "v1",
			SortOrder: 2,
			FilePath:  "content-topics/guides_what_dopamine_does_to_a_teens_brain.txt",
		},
	}

	for _, item := range items {
		contentPath := filepath.Clean(item.FilePath)
		content, err := os.ReadFile(contentPath)
		if err != nil {
			log.Fatal().Err(err).Str("file", contentPath).Msg("cannot read topic content file")
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
			log.Fatal().Err(err).Str("category", item.Category).Str("topic_key", item.TopicKey).Msg("failed to seed topic")
		}

		log.Info().
			Int64("id", doc.ID).
			Str("category", doc.Category).
			Str("topic_key", doc.TopicKey).
			Str("version", doc.Version).
			Time("updated_at", doc.UpdatedAt).
			Msg("content topic seeded successfully")
	}
}

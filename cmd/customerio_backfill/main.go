package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/The-You-School-HeadLamp/headlamp_backend/crm"
	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/service"
	"github.com/The-You-School-HeadLamp/headlamp_backend/util"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type backfillSubject struct {
	UserID     string
	Role       string
	Email      string
	Plan       string
	CreatedAt  time.Time
	LastSeenAt time.Time
}

func main() {
	role := flag.String("role", "all", "Role to backfill: parent, child, all")
	limit := flag.Int("limit", 0, "Max number of users to process (0 = no limit)")
	dryRun := flag.Bool("dry-run", false, "Print planned actions without writing queue events")
	flag.Parse()

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
	customerIOClient := crm.NewCustomerIOClient(
		config.CustomerIOSiteID,
		config.CustomerIOAPIKey,
		config.CustomerIOTrackAPIURL,
		config.CustomerIOWebhookSecret,
		config.ExternalRequestTimeout,
	)
	analyticsService := service.NewAnalyticsService(store, customerIOClient)

	subjects, err := collectSubjects(ctx, connPool, strings.ToLower(strings.TrimSpace(*role)), *limit)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load backfill subjects")
	}
	if len(subjects) == 0 {
		log.Info().Msg("no users found for backfill")
		return
	}

	processed := 0
	failed := 0
	for _, subject := range subjects {
		processed++
		if *dryRun {
			log.Info().Str("user_id", subject.UserID).Str("role", subject.Role).Str("email", subject.Email).Msg("dry-run backfill subject")
			continue
		}

		err := analyticsService.QueueIdentify(ctx, service.IdentifyInput{
			UserID:     subject.UserID,
			Role:       subject.Role,
			Email:      subject.Email,
			Plan:       subject.Plan,
			AppVersion: "backfill",
			Platform:   "backend",
		})
		if err != nil {
			failed++
			log.Error().Err(err).Str("user_id", subject.UserID).Str("role", subject.Role).Msg("failed to queue identify event")
			continue
		}

		if _, err := analyticsService.SyncComputedSegments(ctx, service.SyncSegmentsInput{
			UserID:     subject.UserID,
			Role:       subject.Role,
			Plan:       subject.Plan,
			CreatedAt:  subject.CreatedAt,
			LastSeenAt: subject.LastSeenAt,
		}); err != nil {
			failed++
			log.Error().Err(err).Str("user_id", subject.UserID).Str("role", subject.Role).Msg("failed to sync user segments")
			continue
		}
	}

	log.Info().Int("total", len(subjects)).Int("processed", processed).Int("failed", failed).Int("queued", processed-failed).Msg("customer.io backfill complete")
}

func collectSubjects(ctx context.Context, conn *pgxpool.Pool, role string, limit int) ([]backfillSubject, error) {
	subjects := make([]backfillSubject, 0)

	appendWithLimit := func(items []backfillSubject) {
		for _, item := range items {
			if limit > 0 && len(subjects) >= limit {
				return
			}
			subjects = append(subjects, item)
		}
	}

	switch role {
	case "parent":
		parents, err := listParentSubjects(ctx, conn, limit)
		if err != nil {
			return nil, err
		}
		appendWithLimit(parents)
	case "child":
		children, err := listChildSubjects(ctx, conn, limit)
		if err != nil {
			return nil, err
		}
		appendWithLimit(children)
	case "all", "":
		parents, err := listParentSubjects(ctx, conn, limit)
		if err != nil {
			return nil, err
		}
		appendWithLimit(parents)
		remaining := limit - len(subjects)
		if limit == 0 || remaining > 0 {
			childrenLimit := 0
			if limit > 0 {
				childrenLimit = remaining
			}
			children, err := listChildSubjects(ctx, conn, childrenLimit)
			if err != nil {
				return nil, err
			}
			appendWithLimit(children)
		}
	default:
		return nil, fmt.Errorf("unsupported role %q (use parent, child, or all)", role)
	}

	return subjects, nil
}

func listParentSubjects(ctx context.Context, conn *pgxpool.Pool, limit int) ([]backfillSubject, error) {
	query := `
		SELECT parent_id, email, created_at, updated_at
		FROM parents
		ORDER BY created_at ASC
	`
	args := []any{}
	if limit > 0 {
		query += ` LIMIT $1`
		args = append(args, limit)
	}

	rows, err := conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]backfillSubject, 0)
	for rows.Next() {
		var item backfillSubject
		if err := rows.Scan(&item.UserID, &item.Email, &item.CreatedAt, &item.LastSeenAt); err != nil {
			return nil, err
		}
		item.Role = "parent"
		item.Plan = "free"
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func listChildSubjects(ctx context.Context, conn *pgxpool.Pool, limit int) ([]backfillSubject, error) {
	query := `
		SELECT id, created_at, updated_at
		FROM children
		WHERE deleted_at IS NULL
		ORDER BY created_at ASC
	`
	args := []any{}
	if limit > 0 {
		query += ` LIMIT $1`
		args = append(args, limit)
	}

	rows, err := conn.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]backfillSubject, 0)
	for rows.Next() {
		var item backfillSubject
		if err := rows.Scan(&item.UserID, &item.CreatedAt, &item.LastSeenAt); err != nil {
			return nil, err
		}
		item.Role = "child"
		item.Plan = "free"
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

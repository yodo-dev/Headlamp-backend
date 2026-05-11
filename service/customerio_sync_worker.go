package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/The-You-School-HeadLamp/headlamp_backend/crm"
	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/rs/zerolog/log"
)

type CustomerIOSyncWorker struct {
	store        db.Store
	client       crm.CustomerIOClient
	pollInterval time.Duration
	maxRetries   int32
	cancel       context.CancelFunc
}

func NewCustomerIOSyncWorker(store db.Store, client crm.CustomerIOClient, pollInterval time.Duration, maxRetries int32) *CustomerIOSyncWorker {
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}
	if maxRetries <= 0 {
		maxRetries = 5
	}
	return &CustomerIOSyncWorker{
		store:        store,
		client:       client,
		pollInterval: pollInterval,
		maxRetries:   maxRetries,
	}
}

func (w *CustomerIOSyncWorker) Start(parent context.Context) {
	if w == nil || !w.client.Enabled() {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	go w.loop(ctx)
}

func (w *CustomerIOSyncWorker) Stop() {
	if w != nil && w.cancel != nil {
		w.cancel()
	}
}

func (w *CustomerIOSyncWorker) loop(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		if err := w.runOnce(ctx); err != nil {
			log.Error().Err(err).Msg("customer.io sync worker run failed")
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *CustomerIOSyncWorker) runOnce(ctx context.Context) error {
	items, err := w.store.ListPendingAnalyticsEvents(ctx, 50)
	if err != nil {
		return err
	}

	for _, item := range items {
		if err := w.dispatch(ctx, item); err != nil {
			nextAttemptAt := time.Now().UTC().Add(time.Duration(item.AttemptCount+1) * 15 * time.Second)
			markErr := w.store.MarkAnalyticsEventFailed(ctx, db.MarkAnalyticsEventFailedParams{
				ID:            item.ID,
				LastError:     err.Error(),
				NextAttemptAt: nextAttemptAt,
				MaxAttempts:   w.maxRetries,
			})
			if markErr != nil {
				log.Error().Err(markErr).Str("event_id", item.ID.String()).Msg("failed to mark analytics event failed")
			}
			continue
		}

		if err := w.store.MarkAnalyticsEventSynced(ctx, item.ID); err != nil {
			log.Error().Err(err).Str("event_id", item.ID.String()).Msg("failed to mark analytics event synced")
		}
	}

	return nil
}

func (w *CustomerIOSyncWorker) dispatch(ctx context.Context, item db.AnalyticsEventRecord) error {
	switch item.EventType {
	case "identify":
		var payload identifyPayload
		if err := json.Unmarshal(item.Payload, &payload); err != nil {
			return err
		}
		return w.client.IdentifyUser(ctx, payload.PersonID, payload.Attributes)
	case "track":
		var payload trackPayload
		if err := json.Unmarshal(item.Payload, &payload); err != nil {
			return err
		}
		return w.client.TrackEvent(ctx, payload.PersonID, payload.EventName, payload.Properties, payload.EventTime)
	default:
		return nil
	}
}

package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/The-You-School-HeadLamp/headlamp_backend/crm"
	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog/log"
)

type AnalyticsService struct {
	store            db.Store
	customerIOClient crm.CustomerIOClient
}

type IdentifyInput struct {
	UserID     string
	Role       string
	Email      string
	Plan       string
	AppVersion string
	Platform   string
	DeviceID   string
	PushToken  string
	Locale     string
	Timezone   string
}

type EventInput struct {
	EventID    string
	EventName  string
	EventTime  time.Time
	UserID     string
	Role       string
	SessionID  string
	ChildID    string
	Properties map[string]any
	AppContext map[string]any
}

type SessionStartInput struct {
	SessionID    string
	UserID       string
	Role         string
	ChildID      string
	StartedAt    time.Time
	SourceScreen string
	AppState     string
}

type SessionEndInput struct {
	SessionID       string
	UserID          string
	Role            string
	ChildID         string
	EndedAt         time.Time
	DurationSeconds int
	Reason          string
}

type SyncSegmentsInput struct {
	UserID     string
	Role       string
	Plan       string
	CreatedAt  time.Time
	LastSeenAt time.Time
}

type identifyPayload struct {
	PersonID   string         `json:"person_id"`
	Attributes map[string]any `json:"attributes"`
}

type trackPayload struct {
	PersonID   string         `json:"person_id"`
	EventName  string         `json:"event_name"`
	Properties map[string]any `json:"properties"`
	EventTime  time.Time      `json:"event_time"`
}

func NewAnalyticsService(store db.Store, customerIOClient crm.CustomerIOClient) *AnalyticsService {
	return &AnalyticsService{store: store, customerIOClient: customerIOClient}
}

func normalizeRole(role, childID string) string {
	trimmed := strings.ToLower(strings.TrimSpace(role))
	if trimmed == "parent" || trimmed == "child" {
		return trimmed
	}
	if strings.TrimSpace(childID) != "" {
		return "child"
	}
	return "parent"
}

func buildPersonID(role, userID string) string {
	return normalizeRole(role, "") + ":" + strings.TrimSpace(userID)
}

func (s *AnalyticsService) QueueIdentify(ctx context.Context, in IdentifyInput) error {
	role := normalizeRole(in.Role, "")
	personID := buildPersonID(role, in.UserID)
	now := time.Now().UTC()
	attributes := map[string]any{
		"role": role,
	}
	if in.Email != "" {
		attributes["email"] = strings.TrimSpace(in.Email)
	}
	if in.Plan != "" {
		attributes["plan"] = strings.TrimSpace(in.Plan)
	}
	if in.AppVersion != "" {
		attributes["app_version"] = strings.TrimSpace(in.AppVersion)
	}
	if in.Platform != "" {
		attributes["platform"] = strings.TrimSpace(in.Platform)
	}
	if in.Locale != "" {
		attributes["locale"] = strings.TrimSpace(in.Locale)
	}
	if in.Timezone != "" {
		attributes["timezone"] = strings.TrimSpace(in.Timezone)
	}
	if in.DeviceID != "" {
		attributes["device_id"] = strings.TrimSpace(in.DeviceID)
	}
	if in.PushToken != "" {
		attributes["push_enabled"] = true
	}
	attributes["last_seen_at"] = now.Format(time.RFC3339)

	segments, err := s.syncComputedSegments(ctx, SyncSegmentsInput{
		UserID:     in.UserID,
		Role:       role,
		Plan:       in.Plan,
		LastSeenAt: now,
	})
	if err != nil {
		log.Warn().Err(err).Str("person_id", personID).Msg("analytics identify segment sync failed")
	} else if len(segments) > 0 {
		attributes["segments"] = segments
	}

	payload, err := json.Marshal(identifyPayload{PersonID: personID, Attributes: attributes})
	if err != nil {
		return err
	}

	if err := s.syncDeviceRegistration(ctx, in.UserID, role, in.DeviceID, in.PushToken); err != nil {
		log.Warn().Err(err).Str("user_id", in.UserID).Str("device_id", in.DeviceID).Msg("analytics identify device sync failed")
	}

	_, err = s.store.CreateAnalyticsEvent(ctx, db.CreateAnalyticsEventParams{
		EventType: "identify",
		EventName: "identify",
		PersonID:  personID,
		UserID:    strings.TrimSpace(in.UserID),
		Role:      role,
		EventTime: now,
		Payload:   payload,
	})
	return err
}

func (s *AnalyticsService) SyncComputedSegments(ctx context.Context, in SyncSegmentsInput) ([]string, error) {
	return s.syncComputedSegments(ctx, in)
}

func (s *AnalyticsService) QueueEvent(ctx context.Context, in EventInput) error {
	if in.EventID != "" {
		if _, err := s.store.GetAnalyticsEventBySourceEventID(ctx, strings.TrimSpace(in.EventID)); err == nil {
			return nil
		} else if !errors.Is(err, db.ErrRecordNotFound) {
			return err
		}
	}

	role := normalizeRole(in.Role, in.ChildID)
	personID := buildPersonID(role, in.UserID)
	properties := map[string]any{}
	for key, value := range in.Properties {
		properties[key] = value
	}
	if len(in.AppContext) > 0 {
		properties["app_context"] = in.AppContext
	}

	payload, err := json.Marshal(trackPayload{
		PersonID:   personID,
		EventName:  strings.TrimSpace(in.EventName),
		Properties: properties,
		EventTime:  effectiveTime(in.EventTime),
	})
	if err != nil {
		return err
	}

	_, err = s.store.CreateAnalyticsEvent(ctx, db.CreateAnalyticsEventParams{
		SourceEventID: strings.TrimSpace(in.EventID),
		EventType:     "track",
		EventName:     strings.TrimSpace(in.EventName),
		PersonID:      personID,
		UserID:        strings.TrimSpace(in.UserID),
		Role:          role,
		SessionID:     strings.TrimSpace(in.SessionID),
		ChildID:       strings.TrimSpace(in.ChildID),
		EventTime:     effectiveTime(in.EventTime),
		Payload:       payload,
	})
	return err
}

func (s *AnalyticsService) QueueSessionStart(ctx context.Context, in SessionStartInput) error {
	return s.QueueEvent(ctx, EventInput{
		EventName: "session_started",
		EventTime: in.StartedAt,
		UserID:    in.UserID,
		Role:      in.Role,
		SessionID: in.SessionID,
		ChildID:   in.ChildID,
		Properties: map[string]any{
			"source_screen": strings.TrimSpace(in.SourceScreen),
			"app_state":     strings.TrimSpace(in.AppState),
		},
	})
}

func (s *AnalyticsService) QueueSessionEnd(ctx context.Context, in SessionEndInput) error {
	return s.QueueEvent(ctx, EventInput{
		EventName: "session_ended",
		EventTime: in.EndedAt,
		UserID:    in.UserID,
		Role:      in.Role,
		SessionID: in.SessionID,
		ChildID:   in.ChildID,
		Properties: map[string]any{
			"duration_seconds": in.DurationSeconds,
			"reason":           strings.TrimSpace(in.Reason),
		},
	})
}

func effectiveTime(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}
	return value.UTC()
}

func (s *AnalyticsService) syncDeviceRegistration(ctx context.Context, userID, role, deviceID, pushToken string) error {
	if strings.TrimSpace(deviceID) == "" || strings.TrimSpace(pushToken) == "" {
		return nil
	}
	parsedUserID, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return nil
	}

	existingDevice, err := s.store.GetDeviceByDeviceID(ctx, deviceID)
	if err != nil && !errors.Is(err, db.ErrRecordNotFound) {
		return err
	}

	provider := pgtype.Text{String: "fcm", Valid: true}
	payload := pgtype.Text{String: strings.TrimSpace(pushToken), Valid: true}

	if errors.Is(err, db.ErrRecordNotFound) {
		_, err = s.store.CreateDevice(ctx, db.CreateDeviceParams{
			UserID:    parsedUserID,
			UserType:  role,
			DeviceID:  strings.TrimSpace(deviceID),
			PushToken: payload,
			Provider:  provider,
		})
		return err
	}

	if existingDevice.UserID != parsedUserID {
		if err := s.store.DeleteDeviceByID(ctx, strings.TrimSpace(deviceID)); err != nil {
			return err
		}
		_, err = s.store.CreateDevice(ctx, db.CreateDeviceParams{
			UserID:    parsedUserID,
			UserType:  role,
			DeviceID:  strings.TrimSpace(deviceID),
			PushToken: payload,
			Provider:  provider,
		})
		return err
	}

	_, err = s.store.UpdateDevicePushToken(ctx, db.UpdateDevicePushTokenParams{
		UserID:    parsedUserID,
		DeviceID:  strings.TrimSpace(deviceID),
		PushToken: payload,
		Provider:  provider,
	})
	return err
}

func (s *AnalyticsService) syncComputedSegments(ctx context.Context, in SyncSegmentsInput) ([]string, error) {
	role := normalizeRole(in.Role, "")
	personID := buildPersonID(role, in.UserID)
	if in.LastSeenAt.IsZero() {
		in.LastSeenAt = time.Now().UTC()
	}

	wanted := computeSegmentSet(in.Plan, in.CreatedAt, in.LastSeenAt)
	for _, segmentName := range wanted {
		meta, err := json.Marshal(map[string]any{
			"plan":         strings.TrimSpace(in.Plan),
			"computed_at":  time.Now().UTC().Format(time.RFC3339),
			"created_at":   toRFC3339(in.CreatedAt),
			"last_seen_at": toRFC3339(in.LastSeenAt),
		})
		if err != nil {
			return nil, err
		}
		if _, err := s.store.UpsertUserSegment(ctx, db.UpsertUserSegmentParams{
			PersonID:    personID,
			UserID:      strings.TrimSpace(in.UserID),
			Role:        role,
			SegmentName: segmentName,
			Metadata:    meta,
			Source:      "system",
		}); err != nil {
			return nil, err
		}
	}

	active, err := s.store.ListActiveUserSegments(ctx, personID)
	if err != nil {
		return nil, err
	}
	wantedSet := map[string]struct{}{}
	for _, name := range wanted {
		wantedSet[name] = struct{}{}
	}
	for _, existing := range active {
		if _, ok := wantedSet[existing.SegmentName]; ok {
			continue
		}
		if err := s.store.ExpireUserSegment(ctx, personID, existing.SegmentName); err != nil {
			return nil, err
		}
	}

	return wanted, nil
}

func computeSegmentSet(plan string, createdAt, lastSeenAt time.Time) []string {
	now := time.Now().UTC()
	segments := make([]string, 0, 4)

	normalizedPlan := strings.ToLower(strings.TrimSpace(plan))
	switch normalizedPlan {
	case "", "free", "basic", "starter", "trial":
		segments = append(segments, "plan_free")
	default:
		segments = append(segments, "plan_paid")
	}

	if !createdAt.IsZero() && now.Sub(createdAt.UTC()) <= 14*24*time.Hour {
		segments = append(segments, "new_user_14d")
	}

	if !lastSeenAt.IsZero() {
		delta := now.Sub(lastSeenAt.UTC())
		if delta <= 7*24*time.Hour {
			segments = append(segments, "engaged_7d")
		}
		if delta >= 30*24*time.Hour {
			segments = append(segments, "inactive_30d")
		}
	}

	return segments
}

func toRFC3339(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/gin-gonic/gin"
)

type customerIOWebhookAttribution struct {
	EventType  string
	PersonID   string
	CampaignID string
	MessageID  string
	DeliveryID string
	LinkID     string
	Action     string
	OccurredAt time.Time
}

func (server *Server) handleCustomerIOWebhook(ctx *gin.Context) {
	if server.customerIOClient == nil || !server.customerIOClient.Enabled() {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "customer.io is not configured"})
		return
	}

	body, err := ctx.GetRawData()
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	signature := ctx.GetHeader("X-CIO-Signature")
	if !server.customerIOClient.ValidateWebhookSignature(signature, body) {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "invalid webhook signature"})
		return
	}

	eventType := "unknown"
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err == nil {
		if value, ok := payload["event"].(string); ok && value != "" {
			eventType = value
		} else if value, ok := payload["metric"].(string); ok && value != "" {
			eventType = value
		}
	}

	webhookEvent, err := server.store.CreateCustomerIOWebhookEvent(ctx.Request.Context(), db.CreateCustomerIOWebhookEventParams{
		EventType: eventType,
		Signature: signature,
		Payload:   body,
	})
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	attribution := parseCustomerIOWebhookAttribution(eventType, payload)
	if attribution.EventType != "" {
		if _, err := server.store.CreateCustomerIOAttribution(ctx.Request.Context(), db.CreateCustomerIOAttributionParams{
			WebhookEventID: webhookEvent.ID,
			EventType:      attribution.EventType,
			PersonID:       attribution.PersonID,
			CampaignID:     attribution.CampaignID,
			MessageID:      attribution.MessageID,
			DeliveryID:     attribution.DeliveryID,
			LinkID:         attribution.LinkID,
			Action:         attribution.Action,
			OccurredAt:     attribution.OccurredAt,
			Payload:        body,
		}); err != nil {
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
	}

	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func parseCustomerIOWebhookAttribution(eventType string, payload map[string]any) customerIOWebhookAttribution {
	normalized := strings.ToLower(strings.TrimSpace(eventType))
	if !isTrackedCustomerIOEvent(normalized) {
		return customerIOWebhookAttribution{}
	}

	return customerIOWebhookAttribution{
		EventType:  normalized,
		PersonID:   firstNonEmptyString(fromAny(payload["customer_id"]), fromAny(payload["person_id"]), fromNested(payload, "customer", "id")),
		CampaignID: firstNonEmptyString(fromAny(payload["campaign_id"]), fromNested(payload, "campaign", "id")),
		MessageID:  firstNonEmptyString(fromAny(payload["message_id"]), fromNested(payload, "message", "id")),
		DeliveryID: firstNonEmptyString(fromAny(payload["delivery_id"]), fromNested(payload, "delivery", "id")),
		LinkID:     firstNonEmptyString(fromAny(payload["link_id"]), fromNested(payload, "link", "id")),
		Action:     deriveAction(normalized),
		OccurredAt: parseWebhookTime(payload),
	}
}

func isTrackedCustomerIOEvent(eventType string) bool {
	switch eventType {
	case "delivered", "opened", "clicked", "bounced", "unsubscribed":
		return true
	default:
		return false
	}
}

func deriveAction(eventType string) string {
	switch eventType {
	case "delivered":
		return "delivery"
	case "opened":
		return "open"
	case "clicked":
		return "click"
	case "bounced":
		return "bounce"
	case "unsubscribed":
		return "unsubscribe"
	default:
		return ""
	}
}

func parseWebhookTime(payload map[string]any) time.Time {
	now := time.Now().UTC()
	for _, key := range []string{"timestamp", "event_time", "occurred_at", "created_at"} {
		value, exists := payload[key]
		if !exists || value == nil {
			continue
		}
		switch typed := value.(type) {
		case float64:
			if typed > 0 {
				return time.Unix(int64(typed), 0).UTC()
			}
		case string:
			trimmed := strings.TrimSpace(typed)
			if trimmed == "" {
				continue
			}
			if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
				return parsed.UTC()
			}
		}
	}
	return now
}

func fromNested(payload map[string]any, objectKey, valueKey string) string {
	item, ok := payload[objectKey]
	if !ok || item == nil {
		return ""
	}
	m, ok := item.(map[string]any)
	if !ok {
		return ""
	}
	return fromAny(m[valueKey])
}

func fromAny(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatInt(int64(v), 10)
	default:
		return ""
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

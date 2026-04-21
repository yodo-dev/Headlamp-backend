package api

import (
	"context"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// ─── Notification helpers ────────────────────────────────────────────────────

// notifyParentOfReflectionResponse sends a push notification to the parent when
// their child responds to a reflection. Intended to be called as a goroutine.
func (server *Server) notifyParentOfReflectionResponse(childID string) {
	if server.notificationService == nil {
		return
	}
	ctx := context.Background()
	parentUID, childName, ok := server.lookupParentForChild(ctx, childID)
	if !ok {
		return
	}
	if err := server.notificationService.CreateAndSend(
		ctx,
		parentUID,
		db.NotificationRecipientTypeParent,
		childName+" responded to their reflection",
		"Tap to see what "+childName+" shared today.",
	); err != nil {
		log.Warn().Err(err).Str("child_id", childID).Msg("notify: failed to send reflection response notification")
	}
}

// notifyParentOfReflectionAcknowledge sends a push notification to the parent when
// their child acknowledges a reflection. Intended to be called as a goroutine.
func (server *Server) notifyParentOfReflectionAcknowledge(childID string) {
	if server.notificationService == nil {
		return
	}
	ctx := context.Background()
	parentUID, childName, ok := server.lookupParentForChild(ctx, childID)
	if !ok {
		return
	}
	if err := server.notificationService.CreateAndSend(
		ctx,
		parentUID,
		db.NotificationRecipientTypeParent,
		childName+" read their reflection",
		childName+" acknowledged their reflection today.",
	); err != nil {
		log.Warn().Err(err).Str("child_id", childID).Msg("notify: failed to send reflection acknowledge notification")
	}
}

// notifyChildOfSocialMediaUpdate sends a push notification to a child when their
// social media access settings are changed. Intended to be called as a goroutine.
func (server *Server) notifyChildOfSocialMediaUpdate(childID string) {
	if server.notificationService == nil {
		return
	}
	ctx := context.Background()
	recipientID, err := uuid.Parse(childID)
	if err != nil {
		log.Warn().Str("child_id", childID).Msg("notify: invalid child UUID for social media notification")
		return
	}
	if err := server.notificationService.CreateAndSend(
		ctx,
		recipientID,
		db.NotificationRecipientTypeChild,
		"Your social media settings were updated",
		"Your parent has updated your app access settings.",
	); err != nil {
		log.Warn().Err(err).Str("child_id", childID).Msg("notify: failed to send social media update notification")
	}
}

// notifyParentOfHighRiskContent sends a push notification to the parent when a
// high-severity content monitoring event is ingested. Intended to be called as a goroutine.
func (server *Server) notifyParentOfHighRiskContent(childID string) {
	if server.notificationService == nil {
		return
	}
	ctx := context.Background()
	parentUID, childName, ok := server.lookupParentForChild(ctx, childID)
	if !ok {
		return
	}
	if err := server.notificationService.CreateAndSend(
		ctx,
		parentUID,
		db.NotificationRecipientTypeParent,
		"Risk alert for "+childName,
		"A high-risk content event was detected for "+childName+". Tap to review.",
	); err != nil {
		log.Warn().Err(err).Str("child_id", childID).Msg("notify: failed to send high-risk content notification")
	}
}

// lookupParentForChild returns the parent's UUID and the child's first name given
// a child ID string. Returns (zero, "", false) on any error.
func (server *Server) lookupParentForChild(ctx context.Context, childID string) (uuid.UUID, string, bool) {
	child, err := server.store.GetChild(ctx, childID)
	if err != nil {
		log.Warn().Err(err).Str("child_id", childID).Msg("notify: failed to get child")
		return uuid.UUID{}, "", false
	}

	parent, err := server.store.GetParentByFamilyID(ctx, child.FamilyID)
	if err != nil {
		log.Warn().Err(err).Str("family_id", child.FamilyID).Msg("notify: failed to get parent by family_id")
		return uuid.UUID{}, "", false
	}

	parentUID, err := uuid.Parse(parent.ParentID)
	if err != nil {
		log.Warn().Str("parent_id", parent.ParentID).Msg("notify: parent_id is not a valid UUID")
		return uuid.UUID{}, "", false
	}

	return parentUID, child.FirstName, true
}

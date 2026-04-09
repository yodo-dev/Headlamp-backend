package api

import (
	"errors"
	"fmt"
	"strings"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/The-You-School-HeadLamp/headlamp_backend/token"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

func (server *Server) getAuthPayload(ctx *gin.Context) *token.Payload {
	authPayload, exists := ctx.Get(authorizationPayloadKey)
	if !exists {
		log.Warn().Str("path", ctx.FullPath()).Msg("getAuthPayload: key not found in context")
		return nil
	}

	// Parent / simple auth middleware stores *token.Payload directly
	if payload, ok := authPayload.(*token.Payload); ok {
		return payload
	}

	// deviceAuthMiddleware stores db.Child — convert to a minimal Payload so
	// all child handlers can use getAuthPayload uniformly
	if child, ok := authPayload.(db.Child); ok {
		log.Debug().Str("child_id", child.ID).Str("path", ctx.FullPath()).Msg("getAuthPayload: resolved from db.Child")
		return &token.Payload{
			UserID: child.ID,
			Role:   "child",
		}
	}

	log.Error().Str("path", ctx.FullPath()).Str("type", fmt.Sprintf("%T", authPayload)).Msg("getAuthPayload: unexpected payload type")
	return nil
}

func (server *Server) isParentOfChild(ctx *gin.Context, parentUserID string, childID string) (bool, error) {
	parent, err := server.store.GetParentByParentID(ctx, parentUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil // Parent not found, so can't be parent of child
		}
		return false, err // Some other error
	}

	child, err := server.store.GetChild(ctx, childID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil // Child not found
		}
		return false, err
	}

	return parent.FamilyID == child.FamilyID, nil
}

func flattenDescription(blocks []extRichText) string {
	var b strings.Builder
	for i, blk := range blocks {
		for _, ch := range blk.Children {
			if ch.Text != "" {
				b.WriteString(ch.Text)
			}
		}
		if i < len(blocks)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

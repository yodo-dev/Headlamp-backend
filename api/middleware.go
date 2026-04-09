package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	// "github.com/The-You-School-HeadLamp/headlamp_backend/token"
	// "github.com/The-You-School-HeadLamp/headlamp_backend/token"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

const (
	authorizationHeaderKey  = "authorization"
	authorizationTypeBearer = "bearer"
	authorizationPayloadKey = "authorization_payload"
)

// authMiddleware creates a gin middleware for authorization
func (server *Server) deviceAuthMiddleware() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		authorizationHeader := ctx.GetHeader(authorizationHeaderKey)
		if len(authorizationHeader) == 0 {
			log.Warn().Str("path", ctx.FullPath()).Msg("deviceAuthMiddleware: no authorization header")
			err := errors.New("authorization header is not provided")
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse(err))
			return
		}

		fields := strings.Fields(authorizationHeader)
		if len(fields) < 2 {
			log.Warn().Str("path", ctx.FullPath()).Msg("deviceAuthMiddleware: invalid header format")
			err := errors.New("invalid authorization header format")
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse(err))
			return
		}

		authorizationType := strings.ToLower(fields[0])
		if authorizationType != authorizationTypeBearer {
			log.Warn().Str("path", ctx.FullPath()).Str("type", authorizationType).Msg("deviceAuthMiddleware: unsupported auth type")
			err := fmt.Errorf("unsupported authorization type %s", authorizationType)
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse(err))
			return
		}

		accessToken := fields[1]
		log.Debug().Str("path", ctx.FullPath()).Str("token_prefix", accessToken[:min(20, len(accessToken))]).Msg("deviceAuthMiddleware: verifying token")
		payload, err := server.tokenMaker.VerifyToken(accessToken)
		if err != nil {
			log.Warn().Err(err).Str("path", ctx.FullPath()).Msg("deviceAuthMiddleware: token verification failed")
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse(err))
			return
		}
		log.Debug().Str("user_id", payload.UserID).Str("role", payload.Role).Msg("deviceAuthMiddleware: token verified")

		// Check for role in payload
		if payload.Role != "child" {
			log.Warn().Str("role", payload.Role).Str("path", ctx.FullPath()).Msg("deviceAuthMiddleware: wrong role")
			ctx.AbortWithStatusJSON(http.StatusForbidden, errorResponse(errors.New("user is not a child")))
			return
		}

		// Second factor: Verify the encrypted device ID
		// encryptedDeviceID := ctx.GetHeader(encryptedDeviceIDHeaderKey)

		// if len(encryptedDeviceID) == 0 {
		// 	err := errors.New("x-encrypted-device-id header is not provided")
		// 	ctx.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse(err))
		// 	return
		// }

		child, err := server.store.GetChild(ctx, payload.UserID)
		if err != nil {
			log.Error().Err(err).Str("user_id", payload.UserID).Msg("deviceAuthMiddleware: GetChild failed")
			ctx.AbortWithStatusJSON(http.StatusNotFound, errorResponse(err))
			return
		}
		log.Debug().Str("child_id", child.ID).Str("path", ctx.FullPath()).Msg("deviceAuthMiddleware: child authenticated OK")

		// family, err := server.store.GetFamily(ctx, child.FamilyID)
		// if err != nil {
		// 	ctx.AbortWithStatusJSON(http.StatusNotFound, errorResponse(err))
		// 	return
		// }

		// decodedDeviceID, err := base64.StdEncoding.DecodeString(encryptedDeviceID)
		// if err != nil {
		// 	ctx.AbortWithStatusJSON(http.StatusBadRequest, errorResponse(errors.New("failed to decode device id")))
		// 	return
		// }

		// decryptedDeviceID, err := util.DecryptWithPrivateKey(decodedDeviceID, family.PrivateKey)
		// if err != nil {
		// 	ctx.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse(errors.New("failed to decrypt device id")))
		// 	return
		// }

		// arg := db.GetActiveDeviceByChildAndDeviceIDParams{
		// 	ChildID:  token.UID,
		// 	DeviceID: string(decryptedDeviceID),
		// }
		// _, err = server.store.GetActiveDeviceByChildAndDeviceID(ctx, arg)
		// if err != nil {
		// 	if err == pgx.ErrNoRows {
		// 		ctx.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse(errors.New("device is not active")))
		// 		return
		// 	}
		// 	ctx.AbortWithStatusJSON(http.StatusInternalServerError, errorResponse(err))
		// 	return
		// }

		ctx.Set(authorizationPayloadKey, child)
		ctx.Next()
	}
}

// simpleAuthMiddleware is a simpler auth middleware that just verifies the token
// and attaches the payload to the context.
func (server *Server) simpleAuthMiddleware(allowedRoles ...string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		authorizationHeader := ctx.GetHeader(authorizationHeaderKey)
		if len(authorizationHeader) == 0 {
			err := errors.New("authorization header is not provided")
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse(err))
			return
		}

		fields := strings.Fields(authorizationHeader)
		if len(fields) < 2 {
			err := errors.New("invalid authorization header format")
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse(err))
			return
		}

		authorizationType := strings.ToLower(fields[0])
		if authorizationType != authorizationTypeBearer {
			err := fmt.Errorf("unsupported authorization type %s", authorizationType)
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse(err))
			return
		}

		accessToken := fields[1]
		payload, err := server.tokenMaker.VerifyToken(accessToken)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse(err))
			return
		}

		// Check if the user's role is allowed
		isAllowed := false
		if len(allowedRoles) > 0 {
			for _, role := range allowedRoles {
				if payload.Role == role {
					isAllowed = true
					break
				}
			}
		} else {
			// if no roles are specified, allow any authenticated user
			isAllowed = true
		}

		if !isAllowed {
			err := fmt.Errorf("role %s is not allowed for this route", payload.Role)
			ctx.AbortWithStatusJSON(http.StatusForbidden, errorResponse(err))
			return
		}

		ctx.Set(authorizationPayloadKey, payload)
		ctx.Next()
	}
}

func (server *Server) authMiddleware(requiredRoles ...string) gin.HandlerFunc {
	return func(ctx *gin.Context) {

		// In development mode, we can bypass the actual token verification
		// if strings.ToLower(server.config.Environment) == "development" {
		// 	payload := &token.Payload{
		// 		UserID:   "97293020-1d71-4f7e-8a96-14c13fa85eb5",
		// 		FamilyID: "8dbefdd7-f7b8-4789-a283-1a404bd938e7",
		// 		Role:     "parent",
		// 	}
		// 	ctx.Set(authorizationPayloadKey, payload)
		// 	ctx.Next()
		// 	return
		// }

		authorizationHeader := ctx.GetHeader(authorizationHeaderKey)

		if len(authorizationHeader) == 0 {
			err := errors.New("authorization header is not provided")
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse(err))
			return
		}

		fields := strings.Fields(authorizationHeader)
		if len(fields) < 2 {
			err := errors.New("invalid authorization header format")
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse(err))
			return
		}

		authorizationType := strings.ToLower(fields[0])
		if authorizationType != authorizationTypeBearer {
			err := fmt.Errorf("unsupported authorization type %s", authorizationType)
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse(err))
			return
		}

		accessToken := fields[1]
		payload, err := server.tokenMaker.VerifyToken(accessToken)
		if err != nil {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse(err))
			return
		}

		// Check if the user's role is allowed to access the endpoint.
		if len(requiredRoles) > 0 {
			isAllowed := false
			for _, role := range requiredRoles {
				if payload.Role == role {
					isAllowed = true
					break
				}
			}

			if !isAllowed {
				err := fmt.Errorf("user with role '%s' is not authorized to access this resource", payload.Role)
				ctx.AbortWithStatusJSON(http.StatusForbidden, errorResponse(err))
				return
			}
		}

		ctx.Set(authorizationPayloadKey, payload)

		// If the route involves a specific child, check if the parent has access to this child.
		if childID := ctx.Param("id"); childID != "" {
			child, err := server.store.GetChild(ctx, childID)
			if err != nil {
				ctx.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "child not found"})
				return
			}

			if child.FamilyID != payload.FamilyID {
				ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "you do not have permission to access this child's data"})
				return
			}
		}

		ctx.Next()
	}
}

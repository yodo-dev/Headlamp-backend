package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

var errMobileConfigUnavailable = errors.New("mobile config service is not initialized")

type strapiWebhookPayload struct {
	Event string `json:"event"`
	Model string `json:"model"`
}

func (server *Server) handleStrapiWebhook(ctx *gin.Context) {
	requestLogger := log.With().
		Str("component", "strapi_webhook").
		Str("path", ctx.FullPath()).
		Str("method", ctx.Request.Method).
		Str("remote_ip", ctx.ClientIP()).
		Logger()

	if server.mobileConfigService == nil {
		requestLogger.Error().Msg("mobile config service is not initialized")
		ctx.JSON(http.StatusServiceUnavailable, errorResponse(errMobileConfigUnavailable))
		return
	}

	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		requestLogger.Error().Err(err).Msg("failed to read webhook request body")
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	requestLogger.Info().
		Int("body_bytes", len(body)).
		Bool("signature_present", ctx.GetHeader("X-Strapi-Signature") != "").
		Msg("received Strapi webhook request")

	signature := ctx.GetHeader("X-Strapi-Signature")
	if !server.verifyStrapiSignature(body, signature) {
		requestLogger.Warn().Msg("webhook signature verification failed")
		ctx.JSON(http.StatusUnauthorized, errorResponse(errors.New("invalid webhook signature")))
		return
	}
	requestLogger.Info().Msg("webhook signature verified")

	var payload strapiWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		requestLogger.Error().Err(err).Msg("failed to parse webhook payload")
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	requestLogger = requestLogger.With().
		Str("event", payload.Event).
		Str("model", payload.Model).
		Logger()

	if !isMobileConfigModel(payload.Model) || !isSupportedStrapiEvent(payload.Event) {
		requestLogger.Info().Msg("webhook ignored due to unsupported event/model")
		ctx.JSON(http.StatusOK, gin.H{"status": "ignored"})
		return
	}

	if err := server.mobileConfigService.InvalidateCache(ctx.Request.Context()); err != nil {
		requestLogger.Error().Err(err).Msg("failed to invalidate mobile config cache")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	requestLogger.Info().Msg("mobile config cache invalidated successfully")

	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (server *Server) verifyStrapiSignature(body []byte, signature string) bool {
	secret := strings.TrimSpace(server.config.StrapiWebhookSecret)
	provided := strings.TrimSpace(signature)
	if secret == "" || provided == "" {
		return false
	}

	if hmac.Equal([]byte(provided), []byte(secret)) {
		return true
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sum := mac.Sum(nil)
	candidates := []string{
		hex.EncodeToString(sum),
		"sha256=" + hex.EncodeToString(sum),
		base64.StdEncoding.EncodeToString(sum),
	}

	for _, candidate := range candidates {
		if hmac.Equal([]byte(provided), []byte(candidate)) {
			return true
		}
	}
	return false
}

func isMobileConfigModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "mobile-ui-config" || m == "mobile_ui_config" {
		return true
	}
	return strings.Contains(m, "mobile-ui-config") || strings.Contains(m, "mobile_ui_config")
}

func isSupportedStrapiEvent(event string) bool {
	switch strings.ToLower(strings.TrimSpace(event)) {
	case "entry.create", "entry.update", "entry.publish", "entry.unpublish", "entry.delete":
		return true
	default:
		return false
	}
}

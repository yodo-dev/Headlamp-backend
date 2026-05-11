package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type getMobileConfigRequest struct {
	Locale     string `form:"locale"`
	Local      string `form:"local"`
	Platform   string `form:"platform" binding:"required,oneof=ios android"`
	AppVersion string `form:"app_version" binding:"required"`
}

func (server *Server) getMobileConfig(ctx *gin.Context) {
	if server.mobileConfigService == nil {
		ctx.JSON(http.StatusServiceUnavailable, errorResponse(errMobileConfigUnavailable))
		return
	}

	var req getMobileConfigRequest
	if !bindAndValidateQuery(ctx, &req) {
		return
	}

	locale := strings.TrimSpace(req.Locale)
	if locale == "" {
		locale = strings.TrimSpace(req.Local)
	}
	if locale == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"errors": map[string]string{"Locale": "This field is required"}})
		return
	}

	resp, err := server.mobileConfigService.GetConfig(ctx.Request.Context(), locale, req.Platform, req.AppVersion)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, resp)
}

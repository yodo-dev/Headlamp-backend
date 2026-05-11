package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type getMobileConfigRequest struct {
	Lang       string `form:"lang"`
	Platform   string `form:"platform" binding:"omitempty,oneof=ios android"`
	AppVersion string `form:"app_version"`
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

	locale := strings.TrimSpace(req.Lang)

	resp, err := server.mobileConfigService.GetConfig(ctx.Request.Context(), locale, req.Platform, req.AppVersion)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	ctx.JSON(http.StatusOK, resp)
}

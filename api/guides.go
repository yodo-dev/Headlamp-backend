package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// getGuidesHandler returns all parent guides (PDFs) from Strapi
func (server *Server) getGuidesHandler(ctx *gin.Context) {
	guides, err := server.strapiClient.FetchGuides(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch guides"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"guides": guides})
}

package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/The-You-School-HeadLamp/headlamp_backend/strapi"
	"github.com/gin-gonic/gin"
)

type mockStrapiClient struct {
	guides []strapi.Guide
	err    error
}

func (m mockStrapiClient) FetchMobileUIConfig(_ context.Context, _ string) ([]strapi.Attributes, time.Time, error) {
	return nil, time.Time{}, nil
}

func (m mockStrapiClient) FetchGuides(_ context.Context) ([]strapi.Guide, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.guides, nil
}

func TestGetGuidesHandlerSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := &Server{
		strapiClient: mockStrapiClient{
			guides: []strapi.Guide{
				{ID: 1, Title: "Guide 1", Description: "Desc", PDFUrl: "https://cdn.example.com/guide1.pdf"},
			},
		},
	}

	r := gin.New()
	r.GET("/v1/guides", server.getGuidesHandler)

	req := httptest.NewRequest(http.MethodGet, "/v1/guides", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var payload struct {
		Guides []strapi.Guide `json:"guides"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(payload.Guides) != 1 {
		t.Fatalf("expected 1 guide, got %d", len(payload.Guides))
	}
	if payload.Guides[0].PDFUrl == "" {
		t.Fatal("expected guide pdf_url to be present")
	}
}

func TestGetGuidesHandlerFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := &Server{
		strapiClient: mockStrapiClient{err: errors.New("strapi unavailable")},
	}

	r := gin.New()
	r.GET("/v1/guides", server.getGuidesHandler)

	req := httptest.NewRequest(http.MethodGet, "/v1/guides", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d", http.StatusBadGateway, w.Code)
	}
}

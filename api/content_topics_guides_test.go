package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/The-You-School-HeadLamp/headlamp_backend/strapi"
	"github.com/gin-gonic/gin"
)

func TestListContentTopicsGuidesFromStrapi(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := &Server{
		strapiClient: mockStrapiClient{
			guides: []strapi.Guide{
				{Title: "When Your Kid Says, \"Everyone Else Has a Phone\"", Description: "Guide 1", PDFUrl: "https://cdn.example.com/g1.pdf"},
				{Title: "How to Model Healthy Device Use When You're Struggling Too", Description: "Guide 2", PDFUrl: "https://cdn.example.com/g2.pdf"},
			},
		},
	}

	r := gin.New()
	r.GET("/v1/content/:category", server.listContentTopics)

	req := httptest.NewRequest(http.MethodGet, "/v1/content/guides", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var payload struct {
		Category string `json:"category"`
		Items    []struct {
			TopicKey string `json:"topic_key"`
			PDFURL   string `json:"pdf_url"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if payload.Category != "guides" {
		t.Fatalf("expected category guides, got %s", payload.Category)
	}
	if len(payload.Items) != 2 {
		t.Fatalf("expected 2 guides, got %d", len(payload.Items))
	}
	if payload.Items[0].PDFURL == "" {
		t.Fatal("expected pdf_url in guides list")
	}
}

func TestGetContentTopicDocumentGuidesLegacyTopicKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := &Server{
		strapiClient: mockStrapiClient{
			guides: []strapi.Guide{
				{Title: "When Your Kid Says, \"Everyone Else Has a Phone\"", Description: "Guide 1", PDFUrl: "https://cdn.example.com/g1.pdf"},
			},
		},
	}

	r := gin.New()
	r.GET("/v1/content/:category/:topic_key", server.getContentTopicDocument)

	req := httptest.NewRequest(http.MethodGet, "/v1/content/guides/setting-limits-without-starting-a-war", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var payload struct {
		Category string `json:"category"`
		TopicKey string `json:"topic_key"`
		PDFURL   string `json:"pdf_url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if payload.Category != "guides" {
		t.Fatalf("expected category guides, got %s", payload.Category)
	}
	if payload.PDFURL == "" {
		t.Fatal("expected pdf_url in guide detail response")
	}
}

func TestGetContentTopicDocumentGuidesSecondLegacyTopicKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	server := &Server{
		strapiClient: mockStrapiClient{
			guides: []strapi.Guide{
				{Title: "How to Model Healthy Device Use When You're Struggling Too", Description: "Guide 2", PDFUrl: "https://cdn.example.com/g2.pdf"},
			},
		},
	}

	r := gin.New()
	r.GET("/v1/content/:category/:topic_key", server.getContentTopicDocument)

	req := httptest.NewRequest(http.MethodGet, "/v1/content/guides/what-dopamine-does-to-a-teens-brain", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var payload struct {
		Category string `json:"category"`
		PDFURL   string `json:"pdf_url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if payload.Category != "guides" {
		t.Fatalf("expected category guides, got %s", payload.Category)
	}
	if payload.PDFURL == "" {
		t.Fatal("expected pdf_url in guide detail response")
	}
}

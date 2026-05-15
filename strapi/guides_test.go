package strapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchGuides_FlatPayloadAndPDFOnly(t *testing.T) {
	payload := `{
		"data": [
			{
				"id": 3,
				"text": "When Your Kid Says, \"Everyone Else Has a Phone\"",
				"description": "Guide 1",
				"media": {"url": "/uploads/guide_1.pdf", "mime": "application/pdf"}
			},
			{
				"id": 1,
				"text": "Parent- Critical Thinking",
				"description": "Not a guide PDF",
				"media": {"url": "/uploads/critical_thinking.mp4", "mime": "video/mp4"}
			}
		]
	}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload))
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "token", ts.URL, 5*time.Second).(*client)
	guides, err := c.FetchGuides(context.Background())
	if err != nil {
		t.Fatalf("FetchGuides returned error: %v", err)
	}

	if len(guides) != 1 {
		t.Fatalf("expected 1 PDF guide, got %d", len(guides))
	}
	if guides[0].Title == "" {
		t.Fatal("expected non-empty title")
	}
	if guides[0].PDFUrl == "" {
		t.Fatal("expected non-empty pdf_url")
	}
}

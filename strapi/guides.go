package strapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Guide struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	PDFUrl      string `json:"pdf_url"`
}

type guideListResponse struct {
	Data []map[string]any `json:"data"`
}

func pickGuideString(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		v, ok := obj[key]
		if !ok {
			continue
		}
		s, ok := v.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s != "" {
			return s
		}
	}
	return ""
}

func extractGuideText(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0)
		for _, item := range typed {
			part := extractGuideText(item)
			if part != "" {
				parts = append(parts, part)
			}
		}
		return strings.TrimSpace(strings.Join(parts, " "))
	case map[string]any:
		if text, ok := typed["text"]; ok {
			return extractGuideText(text)
		}
		if children, ok := typed["children"]; ok {
			return extractGuideText(children)
		}
		if data, ok := typed["data"]; ok {
			return extractGuideText(data)
		}
		if attrs, ok := typed["attributes"]; ok {
			return extractGuideText(attrs)
		}
	}
	return ""
}

func extractGuideMedia(value any) (string, string) {
	switch typed := value.(type) {
	case map[string]any:
		if data, ok := typed["data"]; ok {
			if u, m := extractGuideMedia(data); strings.TrimSpace(u) != "" {
				return u, m
			}
		}
		if attrs, ok := typed["attributes"]; ok {
			if u, m := extractGuideMedia(attrs); strings.TrimSpace(u) != "" {
				return u, m
			}
		}
		u := pickGuideString(typed, "url")
		m := pickGuideString(typed, "mime", "mimeType")
		if strings.TrimSpace(u) != "" {
			return u, m
		}
	case []any:
		for _, item := range typed {
			if u, m := extractGuideMedia(item); strings.TrimSpace(u) != "" {
				return u, m
			}
		}
	}
	return "", ""
}

func isPDFGuide(urlValue, mime string) bool {
	lowerURL := strings.ToLower(strings.TrimSpace(urlValue))
	lowerMime := strings.ToLower(strings.TrimSpace(mime))
	return strings.Contains(lowerMime, "pdf") || strings.HasSuffix(lowerURL, ".pdf") || strings.Contains(lowerURL, ".pdf?")
}

func (c *client) FetchGuides(ctx context.Context) ([]Guide, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("strapi base url is not configured")
	}

	endpoint, err := url.Parse(c.baseURL + "/api/parent-resources")
	if err != nil {
		return nil, fmt.Errorf("invalid strapi base url: %w", err)
	}

	q := endpoint.Query()
	q.Set("publicationState", "live")
	q.Set("populate", "*")
	q.Set("pagination[pageSize]", "200")
	endpoint.RawQuery = q.Encode()

	if c.apiToken != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			return nil, fmt.Errorf("strapi request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		var payload guideListResponse
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return nil, err
		}
		guides := make([]Guide, 0, len(payload.Data))
		for _, entry := range payload.Data {
			attrs := entry
			if rawAttrs, ok := entry["attributes"].(map[string]any); ok {
				attrs = rawAttrs
			}

			title := pickGuideString(attrs, "title", "text", "name", "heading")
			if title == "" {
				title = pickGuideString(entry, "title", "text", "name", "heading")
			}

			description := extractGuideText(attrs["description"])
			if description == "" {
				description = pickGuideString(attrs, "description")
			}

			mediaURL, mime := extractGuideMedia(attrs["media"])
			if strings.TrimSpace(mediaURL) == "" {
				mediaURL, mime = extractGuideMedia(entry["media"])
			}
			if strings.TrimSpace(mediaURL) == "" {
				mediaURL, mime = extractGuideMedia(attrs["document"])
			}
			if strings.TrimSpace(mediaURL) == "" {
				mediaURL, mime = extractGuideMedia(entry["document"])
			}

			resolvedURL := c.resolveMediaURL(mediaURL)
			if !isPDFGuide(resolvedURL, mime) {
				continue
			}

			var id int64
			switch typedID := entry["id"].(type) {
			case float64:
				id = int64(typedID)
			case int64:
				id = typedID
			}

			guides = append(guides, Guide{
				ID:          id,
				Title:       strings.TrimSpace(title),
				Description: strings.TrimSpace(description),
				PDFUrl:      resolvedURL,
			})
		}
		return guides, nil
	}
	return nil, fmt.Errorf("Strapi API token not configured")
}

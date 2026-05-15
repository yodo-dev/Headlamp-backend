package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"
)

const parentResourceDocumentKey = "parent_critical_thinking"

type parentResourceMedia struct {
	Name string  `json:"name,omitempty"`
	URL  string  `json:"url,omitempty"`
	Mime string  `json:"mime,omitempty"`
	Size float64 `json:"size,omitempty"`
}

type strapiMediaData struct {
	ID       string
	Title    string
	Video    *parentResourceMedia
	Document *parentResourceMedia
}

type parentResourcePreamble struct {
	Paragraphs []string `json:"paragraphs"`
}

type parentResourceSection struct {
	Heading    string   `json:"heading"`
	Paragraphs []string `json:"paragraphs"`
}

type parentResourceResponse struct {
	ID       string                  `json:"id,omitempty"`
	Title    string                  `json:"title,omitempty"`
	Preamble parentResourcePreamble  `json:"preamble"`
	Sections []parentResourceSection `json:"sections"`
	Video    *parentResourceMedia    `json:"video,omitempty"`
	Document *parentResourceMedia    `json:"document,omitempty"`
}

type strapiParentResourcesEnvelope struct {
	Data any `json:"data"`
}

func (server *Server) getParentResource(ctx *gin.Context) {
	resource, err := server.fetchParentResource(ctx.Request.Context())
	if err != nil {
		log.Error().Err(err).Msg("failed to fetch parent resource")
		if errors.Is(err, pgx.ErrNoRows) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "parent resource not found"})
			return
		}
		ctx.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch parent resource"})
		return
	}
	if resource == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "parent resource not found"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"resource": resource})
}

func (server *Server) fetchParentResource(ctx context.Context) (*parentResourceResponse, error) {
	doc, err := server.store.GetParentResourceDocumentByKey(ctx, parentResourceDocumentKey)
	if err != nil {
		return nil, err
	}

	content := strings.TrimSpace(doc.Content)
	preamble, sections := parseParentResourceContent(content)

	resource := &parentResourceResponse{
		Title:    strings.TrimSpace(doc.Title),
		Preamble: parentResourcePreamble{Paragraphs: preamble},
		Sections: sections,
	}

	mediaResource, err := server.fetchParentResourceMedia(ctx, resource.Title)
	if err != nil {
		return nil, err
	}
	if mediaResource != nil {
		resource.ID = mediaResource.ID
		resource.Video = mediaResource.Video
		resource.Document = mediaResource.Document
		if resource.Title == "" {
			resource.Title = mediaResource.Title
		}
	}

	if resource.Title == "" {
		resource.Title = "Parent- Critical Thinking"
	}

	return resource, nil
}

func parseParentResourceContent(content string) ([]string, []parentResourceSection) {
	lines := strings.Split(content, "\n")
	preambleParagraphs := make([]string, 0)
	sections := make([]parentResourceSection, 0)
	preambleParagraphLines := make([]string, 0)

	var currentSection *parentResourceSection
	currentSectionParagraphLines := make([]string, 0)

	flushPreambleParagraph := func() {
		flushParentResourceParagraph(preambleParagraphLines, &preambleParagraphs)
		preambleParagraphLines = preambleParagraphLines[:0]
	}

	flushSectionParagraph := func() {
		if currentSection == nil {
			return
		}
		flushParentResourceParagraph(currentSectionParagraphLines, &currentSection.Paragraphs)
		currentSectionParagraphLines = currentSectionParagraphLines[:0]
	}

	flushCurrentSection := func() {
		if currentSection == nil {
			return
		}
		flushSectionParagraph()
		sections = append(sections, *currentSection)
		currentSection = nil
	}

	isLikelyHeading := func(trimmed string) bool {
		if len(trimmed) == 0 || len(trimmed) > 150 {
			return false
		}
		if !strings.Contains(trimmed, " ") {
			return false
		}
		if strings.ContainsAny(trimmed, ".?") && !strings.HasPrefix(trimmed, "What ") && !strings.HasPrefix(trimmed, "How ") {
			return false
		}
		return strings.ToUpper(trimmed) != trimmed
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			if currentSection == nil {
				flushPreambleParagraph()
			} else {
				flushSectionParagraph()
			}
			continue
		}

		if isLikelyHeading(trimmed) && len(preambleParagraphLines) > 0 {
			flushPreambleParagraph()
			flushCurrentSection()

			currentSection = &parentResourceSection{
				Heading: trimmed,
			}
			continue
		}

		if currentSection == nil {
			preambleParagraphLines = append(preambleParagraphLines, trimmed)
			continue
		}

		currentSectionParagraphLines = append(currentSectionParagraphLines, trimmed)
	}

	flushPreambleParagraph()
	flushCurrentSection()

	return preambleParagraphs, sections
}

func flushParentResourceParagraph(lines []string, out *[]string) {
	if len(lines) == 0 {
		return
	}
	paragraph := strings.TrimSpace(strings.Join(lines, " "))
	if paragraph == "" {
		return
	}
	*out = append(*out, paragraph)
}

func (server *Server) fetchParentResourceMedia(ctx context.Context, preferredTitle string) (*strapiMediaData, error) {
	base := strings.TrimRight(server.config.ExternalContentBaseURL, "/")
	if base == "" {
		return nil, fmt.Errorf("external content base URL not configured")
	}

	endpoint, err := url.Parse(fmt.Sprintf("%s/api/parent-resources", base))
	if err != nil {
		return nil, fmt.Errorf("parse parent resources URL: %w", err)
	}

	q := endpoint.Query()
	q.Set("publicationState", "live")
	q.Set("sort[0]", "createdAt:desc")
	q.Set("pagination[pageSize]", "25")
	q.Set("populate", "*")
	endpoint.RawQuery = q.Encode()

	timeout := server.config.ExternalRequestTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build parent resource request: %w", err)
	}
	if server.config.ExternalContentToken != "" {
		req.Header.Set("Authorization", "Bearer "+server.config.ExternalContentToken)
	}

	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("parent resource request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("parent resource request status %d", resp.StatusCode)
	}

	var envelope strapiParentResourcesEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("parse parent resource response: %w", err)
	}

	first := pickBestStrapiParentResourceItem(envelope.Data, preferredTitle)
	if first == nil {
		return nil, nil
	}

	resource := &strapiMediaData{
		ID:    pickString(first, "documentId", "id"),
		Title: pickString(first, "title", "name", "heading", "description"),
	}

	labelText := strings.TrimSpace(extractTextFromAny(first["text"]))
	if resource.Title == "" {
		resource.Title = labelText
	}

	if media := extractMedia(first["video"]); media != nil {
		media.URL = server.absoluteURL(media.URL)
		if isLikelyVideoMedia(media) {
			resource.Video = media
		} else if resource.Document == nil {
			resource.Document = media
		}
	}
	if resource.Video == nil {
		if media := extractMedia(first["media"]); media != nil {
			media.URL = server.absoluteURL(media.URL)
			if isLikelyVideoMedia(media) {
				resource.Video = media
			} else if resource.Document == nil {
				resource.Document = media
			}
		}
	}

	if media := extractMedia(first["document"]); media != nil {
		media.URL = server.absoluteURL(media.URL)
		resource.Document = media
	}

	if resource.Document == nil {
		if media := extractMedia(first["doc"]); media != nil {
			media.URL = server.absoluteURL(media.URL)
			resource.Document = media
		}
	}

	if resource.Video == nil && resource.Document != nil && isLikelyVideoMedia(resource.Document) {
		resource.Video = resource.Document
		resource.Document = nil
	}

	return resource, nil
}

func pickBestStrapiParentResourceItem(data any, preferredTitle string) map[string]any {
	items := pickStrapiItems(data)
	if len(items) == 0 {
		return nil
	}

	if len(items) == 1 {
		return items[0]
	}

	normalizedPreferred := normalizeParentResourceLabel(preferredTitle)

	findBy := func(match func(map[string]any) bool) map[string]any {
		for _, item := range items {
			if match(item) {
				return item
			}
		}
		return nil
	}

	if normalizedPreferred != "" {
		if item := findBy(func(item map[string]any) bool {
			label := normalizeParentResourceLabel(parentResourceLabel(item))
			media := extractMedia(item["media"])
			return label == normalizedPreferred && isLikelyVideoMedia(media)
		}); item != nil {
			return item
		}

		if item := findBy(func(item map[string]any) bool {
			label := normalizeParentResourceLabel(parentResourceLabel(item))
			return label == normalizedPreferred
		}); item != nil {
			return item
		}

		if item := findBy(func(item map[string]any) bool {
			label := normalizeParentResourceLabel(parentResourceLabel(item))
			media := extractMedia(item["media"])
			return strings.Contains(label, normalizedPreferred) && isLikelyVideoMedia(media)
		}); item != nil {
			return item
		}

		if item := findBy(func(item map[string]any) bool {
			label := normalizeParentResourceLabel(parentResourceLabel(item))
			return strings.Contains(label, normalizedPreferred)
		}); item != nil {
			return item
		}
	}

	if item := findBy(func(item map[string]any) bool {
		media := extractMedia(item["media"])
		return isLikelyVideoMedia(media)
	}); item != nil {
		return item
	}

	return items[0]
}

func pickStrapiItems(data any) []map[string]any {
	items := make([]map[string]any, 0)

	switch v := data.(type) {
	case []any:
		for _, raw := range v {
			if obj, ok := raw.(map[string]any); ok {
				items = append(items, obj)
			}
		}
	case map[string]any:
		items = append(items, v)
	}

	return items
}

func parentResourceLabel(item map[string]any) string {
	label := pickString(item, "text", "title", "name", "heading", "description")
	if label != "" {
		return label
	}
	return strings.TrimSpace(extractTextFromAny(item["text"]))
}

func normalizeParentResourceLabel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "-", " ")
	value = strings.Join(strings.Fields(value), " ")
	return value
}

func pickFirstStrapiItem(data any) map[string]any {
	switch v := data.(type) {
	case []any:
		if len(v) == 0 {
			return nil
		}
		if obj, ok := v[0].(map[string]any); ok {
			return obj
		}
	case map[string]any:
		return v
	}
	return nil
}

func pickString(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if val, ok := obj[key]; ok {
			if s, ok := val.(string); ok {
				trimmed := strings.TrimSpace(s)
				if trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return ""
}

func extractTextFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0)
		for _, item := range typed {
			piece := strings.TrimSpace(extractTextFromAny(item))
			if piece != "" {
				parts = append(parts, piece)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		if text, ok := typed["text"].(string); ok {
			return strings.TrimSpace(text)
		}
		if children, ok := typed["children"]; ok {
			return extractTextFromAny(children)
		}
		if data, ok := typed["data"]; ok {
			return extractTextFromAny(data)
		}
	}
	return ""
}

func extractMedia(value any) *parentResourceMedia {
	switch typed := value.(type) {
	case map[string]any:
		if data, ok := typed["data"]; ok {
			return extractMedia(data)
		}

		urlValue, _ := typed["url"].(string)
		if strings.TrimSpace(urlValue) == "" {
			return nil
		}

		media := &parentResourceMedia{
			Name: pickString(typed, "name", "fileName"),
			URL:  strings.TrimSpace(urlValue),
			Mime: pickString(typed, "mime", "mimeType"),
		}

		if size, ok := typed["size"].(float64); ok {
			media.Size = size
		}
		if size, ok := typed["sizeInBytes"].(float64); ok && media.Size == 0 {
			media.Size = size
		}

		return media
	case []any:
		if len(typed) == 0 {
			return nil
		}
		return extractMedia(typed[0])
	}

	return nil
}

func isLikelyVideoMedia(media *parentResourceMedia) bool {
	if media == nil {
		return false
	}

	lowerMime := strings.ToLower(strings.TrimSpace(media.Mime))
	if strings.HasPrefix(lowerMime, "video/") {
		return true
	}

	lowerName := strings.ToLower(strings.TrimSpace(media.Name))
	if hasVideoExtension(lowerName) {
		return true
	}

	urlWithoutQuery := strings.TrimSpace(media.URL)
	if idx := strings.Index(urlWithoutQuery, "?"); idx >= 0 {
		urlWithoutQuery = urlWithoutQuery[:idx]
	}
	urlExt := strings.ToLower(path.Ext(urlWithoutQuery))
	return hasVideoExtension(urlExt)
}

func hasVideoExtension(value string) bool {
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	return strings.HasSuffix(lower, ".mp4") ||
		strings.HasSuffix(lower, ".mov") ||
		strings.HasSuffix(lower, ".m4v") ||
		strings.HasSuffix(lower, ".webm") ||
		strings.HasSuffix(lower, ".avi") ||
		strings.HasSuffix(lower, ".mkv") ||
		strings.HasSuffix(lower, ".wmv")
}

package api

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/The-You-School-HeadLamp/headlamp_backend/strapi"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const (
	contentCategoryConversations = "conversations"
	contentCategoryGuides        = "guides"
)

var contentNumberedHeadingPattern = regexp.MustCompile(`^\s*(\d+)\.\s+(.+?)\s*$`)
var contentStepHeadingPattern = regexp.MustCompile(`^\s*Step\s+(\d+)\s*:\s*(.+?)\s*$`)
var genericGuideLabelPattern = regexp.MustCompile(`^guide[\s_-]*\d*$`)

type contentSection struct {
	Number     int      `json:"number"`
	Heading    string   `json:"heading"`
	Paragraphs []string `json:"paragraphs"`
}

type contentPreamble struct {
	Paragraphs []string `json:"paragraphs"`
}

type contentTopicSummary struct {
	TopicKey  string `json:"topic_key"`
	Title     string `json:"title"`
	Subtitle  string `json:"subtitle"`
	Version   string `json:"version"`
	SortOrder int32  `json:"sort_order"`
	UpdatedAt string `json:"updated_at"`
}

type contentTopicListResponse struct {
	Category string                `json:"category"`
	Items    []contentTopicSummary `json:"items"`
}

type contentTopicDetailResponse struct {
	Category  string           `json:"category"`
	TopicKey  string           `json:"topic_key"`
	Title     string           `json:"title"`
	Subtitle  string           `json:"subtitle"`
	Version   string           `json:"version"`
	Preamble  contentPreamble  `json:"preamble"`
	Sections  []contentSection `json:"sections"`
	UpdatedAt string           `json:"updated_at"`
}

type guideTopicSummary struct {
	TopicKey    string `json:"topic_key"`
	Title       string `json:"title"`
	Description string `json:"description"`
	PDFURL      string `json:"pdf_url"`
}

type guideTopicListResponse struct {
	Category string              `json:"category"`
	Items    []guideTopicSummary `json:"items"`
}

type guideTopicDetailResponse struct {
	Category    string `json:"category"`
	TopicKey    string `json:"topic_key"`
	Title       string `json:"title"`
	Description string `json:"description"`
	PDFURL      string `json:"pdf_url"`
}

func guideTopicKeyFromTitle(title string) string {
	normalized := strings.ToLower(strings.TrimSpace(title))

	// Backward-compatible mappings for existing mobile topic keys.
	if strings.Contains(normalized, "everyone else has a phone") {
		return "setting-limits-without-starting-a-war"
	}
	if strings.Contains(normalized, "model healthy device use") {
		return "what-dopamine-does-to-a-teens-brain"
	}
	if strings.Contains(normalized, "readiness assessment") && strings.Contains(normalized, "not yet") {
		return "when-the-readiness-assessment-says-not-yet"
	}

	slug := strings.ToLower(strings.TrimSpace(title))
	slug = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "guide"
	}
	return slug
}

func normalizeGuideDisplayTitle(guide strapi.Guide) string {
	title := strings.TrimSpace(guide.Title)
	description := strings.TrimSpace(guide.Description)

	// Strapi "text" can be generic (Guide_1, Guide_2). Prefer rich description as display title.
	if title == "" || genericGuideLabelPattern.MatchString(strings.ToLower(title)) {
		if description != "" {
			return description
		}
	}

	if title != "" {
		return title
	}

	return description
}

func matchesGuideTopicKey(requestedKey string, guide strapi.Guide) bool {
	requested := strings.TrimSpace(strings.ToLower(requestedKey))
	if requested == "" {
		return false
	}

	titleCandidates := []string{
		normalizeGuideDisplayTitle(guide),
		strings.TrimSpace(guide.Title),
		strings.TrimSpace(guide.Description),
	}

	for _, candidate := range titleCandidates {
		if candidate == "" {
			continue
		}

		canonical := guideTopicKeyFromTitle(candidate)
		if requested == canonical {
			return true
		}

		// Keep accepting both legacy and title-derived keys.
		titleDerived := strings.ToLower(strings.Trim(regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(strings.TrimSpace(candidate)), "-"), "-"))
		if requested == titleDerived {
			return true
		}
	}

	return false
}

func flushContentParagraph(lines []string, out *[]string) {
	if len(lines) == 0 {
		return
	}

	paragraph := strings.TrimSpace(strings.Join(lines, " "))
	if paragraph == "" {
		return
	}

	*out = append(*out, paragraph)
}

func parseStructuredContentSections(content string) ([]string, []contentSection) {
	lines := strings.Split(content, "\n")
	preambleParagraphs := make([]string, 0)
	sections := make([]contentSection, 0)
	preambleParagraphLines := make([]string, 0)

	var currentSection *contentSection
	currentSectionParagraphLines := make([]string, 0)

	flushPreambleParagraph := func() {
		flushContentParagraph(preambleParagraphLines, &preambleParagraphs)
		preambleParagraphLines = preambleParagraphLines[:0]
	}

	flushSectionParagraph := func() {
		if currentSection == nil {
			return
		}
		flushContentParagraph(currentSectionParagraphLines, &currentSection.Paragraphs)
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

		matches := contentNumberedHeadingPattern.FindStringSubmatch(trimmed)
		if len(matches) == 3 {
			flushPreambleParagraph()
			flushCurrentSection()

			number, err := strconv.Atoi(matches[1])
			if err != nil {
				number = 0
			}

			currentSection = &contentSection{
				Number:  number,
				Heading: matches[2],
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

func isConversationHeadingLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}

	if contentStepHeadingPattern.MatchString(trimmed) {
		return true
	}

	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "final thought") {
		return true
	}

	// Conversation headings are usually short standalone lines without ending punctuation.
	if strings.HasSuffix(trimmed, ".") || strings.HasSuffix(trimmed, "?") || strings.HasSuffix(trimmed, "!") || strings.HasSuffix(trimmed, ":") {
		return false
	}

	words := strings.Fields(trimmed)
	if len(words) == 0 || len(words) > 8 {
		return false
	}

	if strings.HasPrefix(lower, "hl parent conversation") {
		return false
	}

	return true
}

func parseConversationContentSections(content string) ([]string, []contentSection) {
	lines := strings.Split(content, "\n")
	preambleParagraphs := make([]string, 0)
	sections := make([]contentSection, 0)

	var currentSection *contentSection
	nextSectionNumber := 1

	flushCurrentSection := func() {
		if currentSection == nil {
			return
		}
		sections = append(sections, *currentSection)
		currentSection = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if isConversationHeadingLine(trimmed) {
			flushCurrentSection()

			heading := trimmed
			number := nextSectionNumber

			if matches := contentStepHeadingPattern.FindStringSubmatch(trimmed); len(matches) == 3 {
				if parsed, err := strconv.Atoi(matches[1]); err == nil {
					number = parsed
				}
				heading = strings.TrimSpace(matches[2])
			}

			currentSection = &contentSection{
				Number:     number,
				Heading:    heading,
				Paragraphs: make([]string, 0),
			}
			nextSectionNumber = number + 1
			continue
		}

		if currentSection == nil {
			preambleParagraphs = append(preambleParagraphs, trimmed)
			continue
		}

		currentSection.Paragraphs = append(currentSection.Paragraphs, trimmed)
	}

	flushCurrentSection()
	return preambleParagraphs, sections
}

func normalizeContentCategory(raw string) (string, bool) {
	normalized := strings.TrimSpace(strings.ToLower(raw))
	switch normalized {
	case contentCategoryConversations, contentCategoryGuides:
		return normalized, true
	default:
		return "", false
	}
}

func (server *Server) listContentTopics(ctx *gin.Context) {
	category, ok := normalizeContentCategory(ctx.Param("category"))
	if !ok {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid category"})
		return
	}

	if category == contentCategoryGuides {
		guides, err := server.strapiClient.FetchGuides(ctx.Request.Context())
		if err != nil {
			ctx.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch guides"})
			return
		}

		items := make([]guideTopicSummary, 0, len(guides))
		for _, guide := range guides {
			displayTitle := normalizeGuideDisplayTitle(guide)
			items = append(items, guideTopicSummary{
				TopicKey:    guideTopicKeyFromTitle(displayTitle),
				Title:       displayTitle,
				Description: guide.Description,
				PDFURL:      guide.PDFUrl,
			})
		}

		ctx.JSON(http.StatusOK, guideTopicListResponse{
			Category: category,
			Items:    items,
		})
		return
	}

	items, err := server.store.ListContentTopicDocumentsByCategory(ctx.Request.Context(), category)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	summaries := make([]contentTopicSummary, 0, len(items))
	for _, item := range items {
		summaries = append(summaries, contentTopicSummary{
			TopicKey:  item.TopicKey,
			Title:     item.Title,
			Subtitle:  item.Subtitle,
			Version:   item.Version,
			SortOrder: item.SortOrder,
			UpdatedAt: item.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	ctx.JSON(http.StatusOK, contentTopicListResponse{
		Category: category,
		Items:    summaries,
	})
}

func (server *Server) getContentTopicDocument(ctx *gin.Context) {
	category, ok := normalizeContentCategory(ctx.Param("category"))
	if !ok {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid category"})
		return
	}

	topicKey := strings.TrimSpace(ctx.Param("topic_key"))
	if topicKey == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "topic_key is required"})
		return
	}

	if category == contentCategoryGuides {
		guides, err := server.strapiClient.FetchGuides(ctx.Request.Context())
		if err != nil {
			ctx.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch guides"})
			return
		}

		for _, guide := range guides {
			displayTitle := normalizeGuideDisplayTitle(guide)
			if !matchesGuideTopicKey(topicKey, guide) {
				continue
			}

			ctx.JSON(http.StatusOK, guideTopicDetailResponse{
				Category:    category,
				TopicKey:    guideTopicKeyFromTitle(displayTitle),
				Title:       displayTitle,
				Description: guide.Description,
				PDFURL:      guide.PDFUrl,
			})
			return
		}

		ctx.JSON(http.StatusNotFound, gin.H{"error": "content topic not found"})
		return
	}

	doc, err := server.store.GetContentTopicDocumentByCategoryAndTopicKey(ctx.Request.Context(), category, topicKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "content topic not found"})
			return
		}

		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	var preambleParagraphs []string
	var sections []contentSection
	if category == contentCategoryConversations {
		preambleParagraphs, sections = parseConversationContentSections(doc.Content)
	} else {
		preambleParagraphs, sections = parseStructuredContentSections(doc.Content)
	}

	ctx.JSON(http.StatusOK, contentTopicDetailResponse{
		Category: category,
		TopicKey: doc.TopicKey,
		Title:    doc.Title,
		Subtitle: doc.Subtitle,
		Version:  doc.Version,
		Preamble: contentPreamble{
			Paragraphs: preambleParagraphs,
		},
		Sections:  sections,
		UpdatedAt: doc.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	})
}

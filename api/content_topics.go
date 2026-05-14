package api

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

const (
	contentCategoryConversations = "conversations"
	contentCategoryGuides        = "guides"
)

var contentNumberedHeadingPattern = regexp.MustCompile(`^\s*(\d+)\.\s+(.+?)\s*$`)

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

	doc, err := server.store.GetContentTopicDocumentByCategoryAndTopicKey(ctx.Request.Context(), category, topicKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "content topic not found"})
			return
		}

		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	preambleParagraphs, sections := parseStructuredContentSections(doc.Content)

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

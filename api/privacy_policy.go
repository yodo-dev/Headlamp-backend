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

const privacyPolicyDocumentKey = "privacy_policy"

var numberedHeadingPattern = regexp.MustCompile(`^\s*(\d+)\.\s+(.+?)\s*$`)

type privacyPolicySection struct {
	Number     int      `json:"number"`
	Heading    string   `json:"heading"`
	Paragraphs []string `json:"paragraphs"`
}

type privacyPolicyPreamble struct {
	Paragraphs []string `json:"paragraphs"`
}

type privacyPolicyResponse struct {
	DocumentKey string                 `json:"document_key"`
	Title       string                 `json:"title"`
	Version     string                 `json:"version"`
	Preamble    privacyPolicyPreamble  `json:"preamble"`
	Sections    []privacyPolicySection `json:"sections"`
	UpdatedAt   string                 `json:"updated_at"`
}

func flushParagraph(lines []string, out *[]string) {
	if len(lines) == 0 {
		return
	}
	paragraph := strings.TrimSpace(strings.Join(lines, " "))
	if paragraph == "" {
		return
	}
	*out = append(*out, paragraph)
}

func parsePrivacyPolicySections(content string) ([]string, []privacyPolicySection) {
	lines := strings.Split(content, "\n")
	preambleParagraphs := make([]string, 0)
	sections := make([]privacyPolicySection, 0)
	preambleParagraphLines := make([]string, 0)

	var currentSection *privacyPolicySection
	currentSectionParagraphLines := make([]string, 0)

	flushPreambleParagraph := func() {
		flushParagraph(preambleParagraphLines, &preambleParagraphs)
		preambleParagraphLines = preambleParagraphLines[:0]
	}

	flushSectionParagraph := func() {
		if currentSection == nil {
			return
		}
		flushParagraph(currentSectionParagraphLines, &currentSection.Paragraphs)
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

		matches := numberedHeadingPattern.FindStringSubmatch(trimmed)
		if len(matches) == 3 {
			flushPreambleParagraph()
			flushCurrentSection()

			number, err := strconv.Atoi(matches[1])
			if err != nil {
				number = 0
			}

			currentSection = &privacyPolicySection{
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

func (server *Server) getPrivacyPolicy(ctx *gin.Context) {
	doc, err := server.store.GetPrivacyPolicyDocumentByKey(ctx.Request.Context(), privacyPolicyDocumentKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "privacy policy not found"})
			return
		}

		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	preambleParagraphs, sections := parsePrivacyPolicySections(doc.Content)

	ctx.JSON(http.StatusOK, privacyPolicyResponse{
		DocumentKey: doc.DocumentKey,
		Title:       doc.Title,
		Version:     doc.Version,
		Preamble: privacyPolicyPreamble{
			Paragraphs: preambleParagraphs,
		},
		Sections:  sections,
		UpdatedAt: doc.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	})
}

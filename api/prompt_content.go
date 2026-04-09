package api

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// FullContentForPrompt represents the entire aggregated content for the GPT prompt.

type FullContentForPrompt struct {
	Courses []CourseWithFullDetails `json:"courses"`
}

// CourseWithFullDetails represents a course with all its modules and quizzes.
type CourseWithFullDetails struct {
	ID          string                  `json:"id"`
	Title       string                  `json:"title"`
	Description string                  `json:"description"`
	Modules     []ModuleWithFullDetails `json:"modules"`
}

// ModuleWithFullDetails represents a module with its full quiz data.
type ModuleWithFullDetails struct {
	ID          string                `json:"id"`
	Title       string                `json:"title"`
	Description string                `json:"description"`
	Quiz        *extQuizWithQuestions `json:"quiz,omitempty"`
}

// aggregateAllContentForPrompt fetches and aggregates all course content.
func (server *Server) aggregateAllContentForPrompt(ctx *gin.Context) (FullContentForPrompt, error) {
	var fullContent FullContentForPrompt

	// 1. Fetch all courses
	courses, err := server.fetchAllExternalCourses(ctx)
	if err != nil {
		return fullContent, err
	}

	// 2. Iterate through courses to fetch modules and quizzes
	for _, courseItem := range courses {
		courseData, err := server.fetchExternalCourseData(ctx, courseItem.DocumentID)
		if err != nil {
			log.Warn().Err(err).Str("course_id", courseItem.DocumentID).Msg("failed to fetch course data for prompt aggregation")
			continue // Skip this course on error
		}

		fullCourse := CourseWithFullDetails{
			ID:          courseData.DocumentID,
			Title:       courseData.Title,
			Description: flattenDescription(courseData.Description),
		}

		for _, moduleItem := range courseData.Modules {
			moduleData, err := server.fetchExternalModuleData(ctx, moduleItem.DocumentID)
			if err != nil {
				log.Warn().Err(err).Str("module_id", moduleItem.DocumentID).Msg("failed to fetch module data for prompt aggregation")
				continue // Skip this module on error
			}

			fullModule := ModuleWithFullDetails{
				ID:          moduleData.DocumentID,
				Title:       moduleData.Title,
				Description: flattenDescription(moduleData.Description),
			}

			if moduleData.Quiz != nil {
				quizData, err := server.fetchExternalQuizData(ctx, moduleData.Quiz.DocumentID)
				if err != nil {
					log.Warn().Err(err).Str("quiz_id", moduleData.Quiz.DocumentID).Msg("failed to fetch quiz data for prompt aggregation")
				} else {
					fullModule.Quiz = quizData
				}
			}
			fullCourse.Modules = append(fullCourse.Modules, fullModule)
		}
		fullContent.Courses = append(fullContent.Courses, fullCourse)
	}

	return fullContent, nil
}

// formatCurriculumForPrompt converts the aggregated curriculum content into a formatted string for the GPT prompt
func formatCurriculumForPrompt(content FullContentForPrompt) string {
	if len(content.Courses) == 0 {
		return ""
	}

	var result string

	for _, course := range content.Courses {
		result += "**Course: " + course.Title + "**\n"
		if course.Description != "" {
			result += "Description: " + course.Description + "\n"
		}
		result += "\n"

		for _, module := range course.Modules {
			result += "  - **Module: " + module.Title + "**\n"
			if module.Description != "" {
				result += "    Description: " + module.Description + "\n"
			}

			if module.Quiz != nil {
				result += "    Quiz Questions:\n"
				if module.Quiz.Questions != nil {
					for i, q := range module.Quiz.Questions {
						result += fmt.Sprintf("      %d. %s\n", i+1, q.Prompt)
						if len(q.AnswerOptions) > 0 {
							for j, opt := range q.AnswerOptions {
								result += fmt.Sprintf("         %c) %s\n", 'A'+rune(j), opt.Text)
							}
						}
					}
				}
			}
			result += "\n"
		}
	}

	return result
}

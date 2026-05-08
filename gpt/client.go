package gpt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

// GptClient defines the interface for interacting with the OpenAI API.
type GptClient interface {
	GetResponse(systemPrompt string, history []openai.ChatCompletionMessage) (string, error)
	InitiateDigitalPermitTest(ctx context.Context) (*GPTResponse, error)
	ContinueDigitalPermitTest(ctx context.Context, previousInteractions []db.DigitalPermitTestInteraction, userAnswer string) (*GPTResponse, error)
	InitiateDigitalPermitTestV2(ctx context.Context, curriculumContext string) (*GPTResponse, error)
	InitiateDigitalPermitTestV2Stream(ctx context.Context, curriculumContext string, onDelta func(delta string) error) (*GPTResponse, error)
	ContinueDigitalPermitTestV2(ctx context.Context, previousInteractions []db.DigitalPermitTestInteraction, userAnswer string, curriculumContext string) (*GPTResponse, error)
	ContinueDigitalPermitTestV2Stream(ctx context.Context, previousInteractions []db.DigitalPermitTestInteraction, userAnswer string, curriculumContext string, onDelta func(delta string) error) (*GPTResponse, error)
	GenerateDailyReflection(ctx context.Context, childCtx ChildReflectionContext) (*DailyReflectionResponse, error)
	GeneratePostSessionReflection(ctx context.Context, sessCtx PostSessionContext) (*PostSessionReflectionResponse, error)
	// GenerateInsights analyses aggregated child behavioral data and returns
	// structured AI insight cards, trends, and maturity scores.
	GenerateInsights(ctx InsightsContext) (*InsightsGPTResponse, error)
	// GenerateParentInsight produces a daily GPT digest for a parent about
	// their child's last 24 hours of activity.
	GenerateParentInsight(ctx context.Context, insightCtx ParentInsightContext) (*ParentInsightResponse, error)
}

// client implements the GptClient interface.
type client struct {
	openaiClient *openai.Client
}

// NewGptClient creates a new client for interacting with the OpenAI API.
func NewGptClient(apiKey string) GptClient {
	return &client{
		openaiClient: openai.NewClient(apiKey),
	}
}

func (c *client) InitiateDigitalPermitTest(ctx context.Context) (*GPTResponse, error) {
	jsonResponse, err := c.GetResponse(DigitalPermitTestSystemPrompt, []openai.ChatCompletionMessage{})
	if err != nil {
		return nil, err
	}

	var gptResponse GPTResponse
	if err := json.Unmarshal([]byte(jsonResponse), &gptResponse); err != nil {
		return nil, err
	}

	return &gptResponse, nil
}

func (c *client) ContinueDigitalPermitTest(ctx context.Context, previousInteractions []db.DigitalPermitTestInteraction, userAnswer string) (*GPTResponse, error) {
	history := buildConversationHistory(previousInteractions, userAnswer)

	jsonResponse, err := c.GetResponse(DigitalPermitTestSystemPrompt, history)
	if err != nil {
		return nil, err
	}

	var gptResponse GPTResponse
	if err := json.Unmarshal([]byte(jsonResponse), &gptResponse); err != nil {
		return nil, err
	}

	return &gptResponse, nil
}

func (c *client) InitiateDigitalPermitTestV2(ctx context.Context, curriculumContext string) (*GPTResponse, error) {
	systemPrompt := buildDigitalPermitTestSystemPromptV2(curriculumContext)
	jsonResponse, err := c.GetResponse(systemPrompt, []openai.ChatCompletionMessage{})
	if err != nil {
		return nil, err
	}

	var gptResponse GPTResponse
	if err := json.Unmarshal([]byte(jsonResponse), &gptResponse); err != nil {
		return nil, err
	}

	return &gptResponse, nil
}

func (c *client) InitiateDigitalPermitTestV2Stream(ctx context.Context, curriculumContext string, onDelta func(delta string) error) (*GPTResponse, error) {
	systemPrompt := buildDigitalPermitTestSystemPromptV2(curriculumContext)
	jsonResponse, err := c.GetResponseStream(ctx, systemPrompt, []openai.ChatCompletionMessage{}, onDelta)
	if err != nil {
		return nil, err
	}

	var gptResponse GPTResponse
	if err := json.Unmarshal([]byte(jsonResponse), &gptResponse); err != nil {
		return nil, err
	}

	return &gptResponse, nil
}

func (c *client) ContinueDigitalPermitTestV2(ctx context.Context, previousInteractions []db.DigitalPermitTestInteraction, userAnswer string, curriculumContext string) (*GPTResponse, error) {
	history := buildConversationHistory(previousInteractions, userAnswer)
	systemPrompt := buildDigitalPermitTestSystemPromptV2(curriculumContext)

	// log.Info().Str("system_prompt", systemPrompt).Msg("system prompt for digital permit test v2")

	jsonResponse, err := c.GetResponse(systemPrompt, history)
	if err != nil {
		return nil, err
	}

	var gptResponse GPTResponse
	if err := json.Unmarshal([]byte(jsonResponse), &gptResponse); err != nil {
		return nil, err
	}

	return &gptResponse, nil
}

func (c *client) ContinueDigitalPermitTestV2Stream(ctx context.Context, previousInteractions []db.DigitalPermitTestInteraction, userAnswer string, curriculumContext string, onDelta func(delta string) error) (*GPTResponse, error) {
	history := buildConversationHistory(previousInteractions, userAnswer)
	systemPrompt := buildDigitalPermitTestSystemPromptV2(curriculumContext)

	jsonResponse, err := c.GetResponseStream(ctx, systemPrompt, history, onDelta)
	if err != nil {
		return nil, err
	}

	var gptResponse GPTResponse
	if err := json.Unmarshal([]byte(jsonResponse), &gptResponse); err != nil {
		return nil, err
	}

	return &gptResponse, nil
}

func (c *client) GenerateDailyReflection(_ context.Context, childCtx ChildReflectionContext) (*DailyReflectionResponse, error) {
	userPrompt := BuildDailyReflectionUserPrompt(childCtx)
	jsonResponse, err := c.GetResponse(DailyReflectionSystemPrompt, []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: userPrompt},
	})
	if err != nil {
		return nil, err
	}
	var resp DailyReflectionResponse
	if err := json.Unmarshal([]byte(jsonResponse), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *client) GeneratePostSessionReflection(_ context.Context, sessCtx PostSessionContext) (*PostSessionReflectionResponse, error) {
	userPrompt := BuildPostSessionReflectionUserPrompt(sessCtx)
	jsonResponse, err := c.GetResponse(PostSessionReflectionSystemPrompt, []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: userPrompt},
	})
	if err != nil {
		return nil, err
	}
	var resp PostSessionReflectionResponse
	if err := json.Unmarshal([]byte(jsonResponse), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func buildConversationHistory(interactions []db.DigitalPermitTestInteraction, latestAnswer string) []openai.ChatCompletionMessage {
	var history []openai.ChatCompletionMessage

	for i, interaction := range interactions {
		// Append the assistant's question
		if interaction.QuestionText.Valid {
			// In the system prompt, we ask the assistant to return a JSON object.
			// We will simulate that here for the history, so it has the full context of its previous turn.
			assistantResponse := GPTResponse{
				QuestionText:    interaction.QuestionText.String,
				QuestionType:    interaction.QuestionType.String,
				Options:         interaction.QuestionOptions,
				Feedback:        interaction.Feedback.String,
				PointsAwarded:   interaction.PointsAwarded.Float64,
				IsFinalQuestion: interaction.IsFinalQuestion.Bool,
			}
			jsonBytes, err := json.Marshal(assistantResponse)
			// This should not fail, but we handle it just in case.
			if err == nil {
				history = append(history, openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleAssistant,
					Content: string(jsonBytes),
				})
			}
		}

		// Append the user's answer
		if i == len(interactions)-1 {
			// This is the most recent interaction, so we use the fresh answer from the current request.
			history = append(history, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: latestAnswer,
			})
		} else if interaction.AnswerText.Valid {
			// For all previous interactions, use the answer stored in the database.
			history = append(history, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: interaction.AnswerText.String,
			})
		}
	}

	return history
}

// modelFallbackOrder defines the primary model and fallbacks in priority order.
var modelFallbackOrder = []string{
	openai.GPT5Mini,
	openai.GPT4Dot1Mini,
	openai.GPT4oMini,
}

// GetResponse sends a system prompt and conversation history to the OpenAI API
// and returns the content of the AI's response as a string.
// It tries models in fallback order and logs which model succeeded.
func (c *client) GetResponse(systemPrompt string, history []openai.ChatCompletionMessage) (string, error) {
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		},
	}
	messages = append(messages, history...)

	var lastErr error
	for _, model := range modelFallbackOrder {
		resp, err := c.openaiClient.CreateChatCompletion(
			context.Background(),
			openai.ChatCompletionRequest{
				Model:          model,
				Messages:       messages,
				ResponseFormat: &openai.ChatCompletionResponseFormat{Type: openai.ChatCompletionResponseFormatTypeJSONObject},
			},
		)
		if err != nil {
			log.Warn().Str("model", model).Err(err).Msg("openai model failed, trying next fallback")
			lastErr = err
			continue
		}
		if len(resp.Choices) == 0 {
			lastErr = errors.New("no response choices from OpenAI")
			log.Warn().Str("model", model).Msg("openai returned no choices, trying next fallback")
			continue
		}
		log.Info().Str("model", model).Msg("openai request succeeded")
		return resp.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("all models failed, last error: %w", lastErr)
}

// GetResponseStream sends a system prompt and conversation history to the OpenAI API
// and streams response deltas through onDelta while collecting the final JSON string.
func (c *client) GetResponseStream(ctx context.Context, systemPrompt string, history []openai.ChatCompletionMessage, onDelta func(delta string) error) (string, error) {
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		},
	}
	messages = append(messages, history...)

	var lastErr error
	for _, model := range modelFallbackOrder {
		stream, err := c.openaiClient.CreateChatCompletionStream(
			ctx,
			openai.ChatCompletionRequest{
				Model:          model,
				Messages:       messages,
				ResponseFormat: &openai.ChatCompletionResponseFormat{Type: openai.ChatCompletionResponseFormatTypeJSONObject},
			},
		)
		if err != nil {
			log.Warn().Str("model", model).Err(err).Msg("openai stream model failed, trying next fallback")
			lastErr = err
			continue
		}

		var content string
		for {
			resp, recvErr := stream.Recv()
			if errors.Is(recvErr, io.EOF) {
				break
			}
			if recvErr != nil {
				lastErr = recvErr
				log.Warn().Str("model", model).Err(recvErr).Msg("openai stream recv failed, trying next fallback")
				break
			}

			if len(resp.Choices) == 0 {
				continue
			}

			delta := resp.Choices[0].Delta.Content
			if delta == "" {
				continue
			}

			content += delta
			if onDelta != nil {
				if cbErr := onDelta(delta); cbErr != nil {
					_ = stream.Close()
					return "", cbErr
				}
			}
		}

		_ = stream.Close()

		if content == "" {
			if lastErr == nil {
				lastErr = errors.New("empty streamed content from OpenAI")
			}
			continue
		}

		log.Info().Str("model", model).Msg("openai stream request succeeded")
		return content, nil
	}

	return "", fmt.Errorf("all stream models failed, last error: %w", lastErr)
}

// buildSingleUserMessage wraps a single text string as a user chat message slice.
func buildSingleUserMessage(content string) []openai.ChatCompletionMessage {
	return []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: content},
	}
}

// GenerateParentInsight calls GPT to produce a daily parent digest for the given child context.
func (c *client) GenerateParentInsight(ctx context.Context, insightCtx ParentInsightContext) (*ParentInsightResponse, error) {
	userPrompt := BuildParentInsightUserPrompt(insightCtx)
	log.Info().Str("child_id", insightCtx.ChildID).Msg("generating parent daily insight")

	jsonResponse, err := c.GetResponse(ParentInsightSystemPrompt, buildSingleUserMessage(userPrompt))
	if err != nil {
		return nil, err
	}

	var result ParentInsightResponse
	if err := json.Unmarshal([]byte(jsonResponse), &result); err != nil {
		return nil, fmt.Errorf("failed to parse parent insight response: %w", err)
	}

	return &result, nil
}

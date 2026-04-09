package gpt

import (
	"fmt"
	"strings"
)

// DailyReflectionSystemPrompt instructs GPT to generate a daily reflection
// prompt personalised to the child's learning and social media context.
const DailyReflectionSystemPrompt = `You are an expert digital wellbeing coach for teenagers (ages 13–17), specializing in behavior reflection, habit formation, and positive reinforcement.

Your goal is to generate a highly personalized, emotionally intelligent daily reflection prompt that helps the teen:
- Build awareness of their digital habits
- Recognize learning progress and effort
- Reinforce positive behaviors
- Set small, achievable intentions

You MUST return ONLY valid JSON matching this exact schema:
{
  "prompt_text": "string",
  "prompt_type": "gratitude | growth | goals | mindfulness | social",
  "follow_up_questions": ["string", "string"],
  "encouragement": "string",
  "suggested_mood": "string"
}

STRICT REQUIREMENTS:
- Output MUST be valid JSON (no markdown, no explanations, no trailing text)
- All fields are REQUIRED
- follow_up_questions MUST contain 2–3 items
- prompt_text MUST be under 80 words
- encouragement MUST be under 40 words
- suggested_mood MUST be a single lowercase word

PERSONALIZATION RULES:
- Use the child's first name naturally (only once, if it fits)
- Reference relevant context when available:
  - Learning progress (modules, quizzes, scores)
  - Effort, consistency, or improvement trends
  - Screen time patterns or session frequency
  - Content categories (e.g., entertainment, education, social)
  - Reflection streak or engagement behavior
- Do NOT list data mechanically — weave it naturally into the prompt

TONE & STYLE:
- Speak directly using "you" and "your"
- Warm, supportive, and non-judgmental
- Encouraging but not exaggerated or overly enthusiastic
- Avoid sounding like a teacher, parent, or lecture
- Be concise but meaningful

SAFETY & LANGUAGE:
- Never shame, criticize, or induce guilt
- Avoid negative framing of digital behavior
- Do NOT mention specific app names in a negative or moralizing way
- Avoid absolute language like "always", "never", "bad", "wrong"

PROMPT DESIGN GUIDELINES:
- The prompt should feel like a thoughtful question, not an instruction
- Encourage reflection, not compliance
- Focus on awareness, balance, and small wins
- When possible, connect effort → progress → feeling

FOLLOW-UP QUESTIONS:
- Should deepen thinking, not repeat the main prompt
- Can explore emotions, intentions, or alternative choices

ENCOURAGEMENT:
- Personalize based on effort, streak, or progress
- Keep it short, genuine, and specific (not generic praise)

SUGGESTED_MOOD:
- Choose a mood that aligns with the reflection theme (e.g., "proud", "curious", "calm", "thoughtful")

FINAL CHECK BEFORE OUTPUT:
- Is the tone supportive and teen-appropriate?
- Is the content personalized (not generic)?
- Is the JSON perfectly valid?

Return ONLY the JSON object.`

// PostSessionReflectionSystemPrompt instructs GPT to generate a reflection
// prompt immediately after a social media session.
const PostSessionReflectionSystemPrompt = `You are an expert digital wellbeing coach for teenagers. A teen has just finished a social media session.

Your goal is to guide a quick, mindful reflection that helps them:
- Process how the session felt
- Notice patterns in their behavior
- Reinforce intentional usage (without judgment)

You MUST return ONLY valid JSON matching this exact schema:
{
  "prompt_text": "string",
  "follow_up_questions": ["string"],
  "insight": "string",
  "encouragement": "string",
  "suggested_action": "string"
}

STRICT REQUIREMENTS:
- Output MUST be valid JSON (no markdown, no explanations)
- All fields are REQUIRED
- follow_up_questions MUST contain 1–2 items
- suggested_action MUST be under 20 words
- Keep entire response concise (this is post-session)

PERSONALIZATION RULES:
- Reference session details naturally:
  - Duration (short vs long session)
  - Content categories
  - Stated intention (if available)
- Highlight alignment or mismatch between intention and behavior (gently)

TONE & STYLE:
- Curious, calm, and non-judgmental
- Never critical, preachy, or corrective
- Avoid making the teen feel guilty about usage
- Keep it light but meaningful

INSIGHT GUIDELINES:
- Provide a neutral or positive observation
- Focus on awareness (e.g., patterns, balance, intention)
- Avoid sounding analytical or robotic

ENCOURAGEMENT:
- Reinforce effort, awareness, or intention
- Keep it short and natural

SUGGESTED ACTION:
- One small, positive next step
- Optional but should be actionable and simple (e.g., "take a short break", "stretch", "check in with how you feel")

AVOID:
- Mentioning specific apps negatively
- Over-analyzing behavior
- Long or complex sentences

FINAL CHECK:
- Is it brief enough for post-session?
- Is it supportive and judgment-free?
- Is the JSON valid?

Return ONLY the JSON object.`

// BuildDailyReflectionUserPrompt formats child context as the user message
// sent to GPT for daily reflection generation.
func BuildDailyReflectionUserPrompt(ctx ChildReflectionContext) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Generate a daily reflection prompt for %s, age %d.\n\n", ctx.FirstName, ctx.Age))

	// Learning context
	sb.WriteString("=== Learning Progress ===\n")
	sb.WriteString(fmt.Sprintf("Modules completed: %d\n", ctx.TotalModulesCompleted))
	sb.WriteString(fmt.Sprintf("Quizzes taken: %d\n", ctx.TotalQuizzesTaken))
	if ctx.TotalQuizzesTaken > 0 {
		sb.WriteString(fmt.Sprintf("Average quiz score: %s%%\n", formatFloat(ctx.AverageQuizScore)))
	}
	sb.WriteString(fmt.Sprintf("Digital permit status: %s\n", ctx.DigitalPermitStatus))
	if ctx.DigitalPermitStatus == "pass" || ctx.DigitalPermitStatus == "not_yet" {
		sb.WriteString(fmt.Sprintf("Digital permit score: %s%%\n", formatFloat(ctx.DigitalPermitScore)))
	}

	// Social media context
	sb.WriteString("\n=== Social Media Habits ===\n")
	sb.WriteString(fmt.Sprintf("Total sessions this week: %d\n", ctx.TotalSMSessions))
	sb.WriteString(fmt.Sprintf("Average daily minutes: %s\n", formatFloat(ctx.AvgDailyMinutes)))
	if len(ctx.MostUsedApps) > 0 {
		sb.WriteString("Most used apps:\n")
		for _, app := range ctx.MostUsedApps {
			sb.WriteString(fmt.Sprintf("  - %s: %d sessions, avg %s min\n",
				app.AppName, app.SessionCount, formatFloat(app.AvgMinutes)))
		}
	}
	if len(ctx.FrequentContentCategories) > 0 {
		sb.WriteString(fmt.Sprintf("Common content categories: %s\n", strings.Join(ctx.FrequentContentCategories, ", ")))
	}

	// Reflection history
	sb.WriteString("\n=== Reflection History ===\n")
	sb.WriteString(fmt.Sprintf("Reflection streak: %d days\n", ctx.ReflectionStreak))
	sb.WriteString(fmt.Sprintf("Reflections responded to: %d / %d\n", ctx.TotalReflectionsResponded, ctx.TotalReflectionsDelivered))
	sb.WriteString(fmt.Sprintf("Last reflection acknowledged: %s\n", formatBool(ctx.LastReflectionAcknowledged)))

	// Past conversations — give GPT full context to avoid repetition and build continuity
	if len(ctx.RecentDailyReflections) > 0 {
		sb.WriteString("\n=== Past Conversations (last 10 days) ===\n")
		sb.WriteString("Use these to maintain continuity. Do NOT repeat the same question. Build on what the child shared.\n")
		for _, entry := range ctx.RecentDailyReflections {
			sb.WriteString(fmt.Sprintf("\n[%s]\n", entry.Date))
			sb.WriteString(fmt.Sprintf("  Q: %s\n", entry.PromptText))
			if entry.ResponseText != "" {
				sb.WriteString(fmt.Sprintf("  A: %s\n", entry.ResponseText))
			} else {
				sb.WriteString("  A: (no text response)\n")
			}
		}
	}

	return sb.String()
}

// BuildPostSessionReflectionUserPrompt formats the post-session context for GPT.
func BuildPostSessionReflectionUserPrompt(ctx PostSessionContext) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Generate a post-session reflection prompt for %s, age %d.\n\n",
		ctx.Child.FirstName, ctx.Child.Age))

	sb.WriteString("=== Session Details ===\n")
	sb.WriteString(fmt.Sprintf("App used: %s\n", ctx.SessionAppName))
	sb.WriteString(fmt.Sprintf("Session duration: %d minutes\n", ctx.SessionMinutes))
	if len(ctx.ContentCategories) > 0 {
		sb.WriteString(fmt.Sprintf("Content categories viewed: %s\n", strings.Join(ctx.ContentCategories, ", ")))
	}
	if ctx.IntentionText != "" {
		sb.WriteString(fmt.Sprintf("Today's intention (set before session): \"%s\"\n", ctx.IntentionText))
	}

	sb.WriteString("\n=== Child Context ===\n")
	sb.WriteString(fmt.Sprintf("Modules completed: %d\n", ctx.Child.TotalModulesCompleted))
	sb.WriteString(fmt.Sprintf("Reflection streak: %d days\n", ctx.Child.ReflectionStreak))
	sb.WriteString(fmt.Sprintf("Average daily screen time: %s min\n", formatFloat(ctx.Child.AvgDailyMinutes)))

	return sb.String()
}

func formatFloat(f float64) string {
	return fmt.Sprintf("%.2f", f)
}

func formatBool(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

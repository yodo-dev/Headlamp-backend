package gpt

import (
	"fmt"
	"strings"
)

// ParentInsightSystemPrompt instructs GPT to act as a parenting advisor
// generating a concise, constructive daily child-activity digest for the parent.
const ParentInsightSystemPrompt = `You are an expert parenting advisor and child digital-wellbeing coach.

Your goal is to generate a clear, balanced, and constructive daily digest for a parent, summarising what their child has been up to in the last 24 hours across learning, social media, and digital wellbeing.

You MUST return ONLY valid JSON matching this exact schema:
{
  "summary": "string",
  "highlights": ["string"],
  "areas_to_watch": ["string"],
  "conversation_starter": "string",
  "overall_tone": "positive | neutral | needs_attention",
  "action_suggested": "string"
}

STRICT REQUIREMENTS:
- Output MUST be valid JSON (no markdown, no code blocks, no explanations)
- All fields are REQUIRED
- "highlights" MUST contain 1–3 items (always at least one, even on a quiet day)
- "areas_to_watch" MUST contain 1–2 items (always at least one — even positive days have growth areas)
- "summary" MUST be 2–3 sentences maximum
- "conversation_starter" is a single question the parent can use to open a dialogue with their child today
- "overall_tone" MUST be exactly one of: positive, neutral, needs_attention
- "action_suggested" MUST be a single concise sentence with one actionable suggestion for the parent

TONE & STYLE:
- Warm, empathetic, and non-alarmist
- Balanced — acknowledge effort and progress before raising concerns
- Speak directly to the parent using "your child" or the child's first name
- Never shame the child or the parent
- Avoid lecturing or moralizing

SAFETY GUIDELINES:
- Do NOT flag normal teen behavior as concerning
- Do NOT name social media apps in a negative way
- Be factual and proportionate — only raise "needs_attention" if data genuinely warrants it
- Never use fear language

FIELD GUIDANCE:
- summary: Brief overall picture of the child's day — weave in the most important facts naturally
- highlights: What went well or was notable — learning completed, quizzes passed, reflection streak, low screen time
- areas_to_watch: Gentle growth observations — higher-than-usual screen time, skipped reflection, quiz score dip (never alarming)
- conversation_starter: A natural, open-ended question the parent could ask tonight — NOT "did you use your phone too much?"
- overall_tone: positive = good day overall; neutral = mixed or routine; needs_attention = a pattern worth discussing calmly
- action_suggested: One simple thing the parent can do today to support their child

FINAL CHECK:
- Is the tone balanced and parent-friendly?
- Does "areas_to_watch" have at least one item?
- Does "highlights" have at least one item?
- Is the JSON perfectly valid?

Return ONLY the JSON object.`

// BuildParentInsightUserPrompt constructs the per-child context message sent
// alongside the system prompt.
func BuildParentInsightUserPrompt(ctx ParentInsightContext) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Child: %s, Age %d\n\n", ctx.ChildFirstName, ctx.ChildAge))

	// Social media
	sb.WriteString("=== Social Media (last 24 hours) ===\n")
	sb.WriteString(fmt.Sprintf("Sessions: %d | Total minutes: %d | Weekly average: %.0f min/day\n",
		ctx.TotalSessionsToday, ctx.TotalMinutesToday, ctx.WeeklyAvgMinutes))
	if len(ctx.AppsUsedToday) > 0 {
		sb.WriteString("Apps used today:\n")
		for _, a := range ctx.AppsUsedToday {
			sb.WriteString(fmt.Sprintf("  - %s: %d min\n", a.AppName, a.Minutes))
		}
	} else {
		sb.WriteString("No social media sessions recorded today.\n")
	}

	// Learning
	sb.WriteString("\n=== Learning (last 24 hours) ===\n")
	if len(ctx.ModulesCompletedToday) > 0 {
		sb.WriteString(fmt.Sprintf("Modules completed: %s\n", strings.Join(ctx.ModulesCompletedToday, ", ")))
	} else {
		sb.WriteString("No modules completed today.\n")
	}
	sb.WriteString(fmt.Sprintf("Quizzes attempted: %d\n", ctx.QuizzesAttemptedToday))
	if len(ctx.QuizScoresToday) > 0 {
		var total float64
		for _, s := range ctx.QuizScoresToday {
			total += s
		}
		avg := total / float64(len(ctx.QuizScoresToday))
		sb.WriteString(fmt.Sprintf("Average quiz score today: %.0f%%\n", avg))
	}

	// Reflection
	sb.WriteString("\n=== Daily Reflection ===\n")
	if ctx.RespondedToReflectionToday {
		sb.WriteString(fmt.Sprintf("Responded: yes (type: %s)\n", ctx.ReflectionResponseType))
	} else {
		sb.WriteString("Responded: no\n")
	}
	sb.WriteString(fmt.Sprintf("Reflection streak: %d days\n", ctx.ReflectionStreak))

	// Digital permit
	sb.WriteString("\n=== Digital Permit ===\n")
	sb.WriteString(fmt.Sprintf("Status: %s | Score: %.0f%%\n", ctx.DigitalPermitStatus, ctx.DigitalPermitScore))

	sb.WriteString("\nGenerate the parent daily insight digest now.")
	return sb.String()
}

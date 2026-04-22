package gpt

// ParentInsightContext holds all data needed to generate a GPT daily digest
// for the parent about a specific child's last 24 hours.
type ParentInsightContext struct {
	ChildID        string `json:"child_id"`
	ChildFirstName string `json:"child_first_name"`
	ChildAge       int    `json:"child_age"`

	// Social media — last 24 hours
	TotalSessionsToday int             `json:"total_sessions_today"`
	TotalMinutesToday  int             `json:"total_minutes_today"`
	AppsUsedToday      []AppUsageToday `json:"apps_used_today"`
	WeeklyAvgMinutes   float64         `json:"weekly_avg_minutes"`

	// Learning — last 24 hours
	ModulesCompletedToday []string  `json:"modules_completed_today"`
	QuizzesAttemptedToday int       `json:"quizzes_attempted_today"`
	QuizScoresToday       []float64 `json:"quiz_scores_today"`

	// Reflection
	RespondedToReflectionToday bool   `json:"responded_to_reflection_today"`
	ReflectionResponseType     string `json:"reflection_response_type"` // "text" | "video" | ""
	ReflectionStreak           int    `json:"reflection_streak"`

	// Digital permit
	DigitalPermitStatus string  `json:"digital_permit_status"` // "pass" | "not_yet" | "not_started"
	DigitalPermitScore  float64 `json:"digital_permit_score"`
}

// AppUsageToday represents a single app's usage data for the last 24 hours.
type AppUsageToday struct {
	AppName string `json:"app_name"`
	Minutes int    `json:"minutes"`
}

// ParentInsightResponse is the structured GPT output for a parent daily digest.
type ParentInsightResponse struct {
	Summary             string   `json:"summary"`
	Highlights          []string `json:"highlights"`
	AreasToWatch        []string `json:"areas_to_watch"`
	ConversationStarter string   `json:"conversation_starter"`
	OverallTone         string   `json:"overall_tone"` // positive | neutral | needs_attention
	ActionSuggested     string   `json:"action_suggested"`
}

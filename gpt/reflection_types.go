package gpt

// ChildReflectionContext contains all contextual data used to personalise
// GPT reflection prompts for a child.
type ChildReflectionContext struct {
	ChildID   string `json:"child_id"`
	FirstName string `json:"first_name"`
	Age       int    `json:"age"`

	// Learning progress
	TotalModulesCompleted int      `json:"total_modules_completed"`
	TotalQuizzesTaken     int      `json:"total_quizzes_taken"`
	AverageQuizScore      float64  `json:"average_quiz_score"`
	CompletedModuleIDs    []string `json:"completed_module_ids"`

	// Digital permit
	DigitalPermitStatus string  `json:"digital_permit_status"` // "pass", "not_yet", "not_started"
	DigitalPermitScore  float64 `json:"digital_permit_score"`

	// Social media habits
	TotalSMSessions           int        `json:"total_sm_sessions"`
	AvgDailyMinutes           float64    `json:"avg_daily_minutes"`
	MostUsedApps              []AppUsage `json:"most_used_apps"`
	FrequentContentCategories []string   `json:"frequent_content_categories"`

	// Recent activities
	RecentActivities []Activity `json:"recent_activities"`

	// Reflection history
	ReflectionStreak           int  `json:"reflection_streak"`
	TotalReflectionsResponded  int  `json:"total_reflections_responded"`
	TotalReflectionsDelivered  int  `json:"total_reflections_delivered"`
	LastReflectionAcknowledged bool `json:"last_reflection_acknowledged"`

	// Past 10 days of daily reflections with the child's responses.
	// Used to maintain conversational continuity across days.
	RecentDailyReflections []PastReflectionEntry `json:"recent_daily_reflections"`
}

// Activity represents a recent action the child took in the app.
type Activity struct {
	Type  string `json:"type"`
	RefID string `json:"ref_id"`
	Date  string `json:"date"` // RFC3339
}

// AppUsage summarises usage of a single social media app.
type AppUsage struct {
	AppName      string  `json:"app_name"`
	SessionCount int     `json:"session_count"`
	AvgMinutes   float64 `json:"avg_minutes"`
}

// PastReflectionEntry holds a single day's question and the child's response,
// used to give GPT conversational context across sessions.
type PastReflectionEntry struct {
	Date         string `json:"date"`          // YYYY-MM-DD
	PromptText   string `json:"prompt_text"`   // the question GPT asked
	ResponseText string `json:"response_text"` // what the child answered
}

// PostSessionContext is used to generate reflections immediately after a
// social media session ends.
type PostSessionContext struct {
	Child             ChildReflectionContext `json:"child"`
	SessionAppName    string                 `json:"session_app_name"`
	SessionMinutes    int                    `json:"session_minutes"`
	ContentCategories []string               `json:"content_categories"`
	IntentionText     string                 `json:"intention_text"` // today's intention, if any
}

// DailyReflectionResponse is the structured JSON output from GPT for a daily
// scheduled reflection prompt.
type DailyReflectionResponse struct {
	PromptText        string   `json:"prompt_text"`
	PromptType        string   `json:"prompt_type"` // e.g. "gratitude", "growth", "goals"
	FollowUpQuestions []string `json:"follow_up_questions"`
	Encouragement     string   `json:"encouragement"`
	SuggestedMood     string   `json:"suggested_mood"` // optional mood check-in label
}

// PostSessionReflectionResponse is the structured JSON output from GPT for a
// post-session prompt.
type PostSessionReflectionResponse struct {
	PromptText        string   `json:"prompt_text"`
	FollowUpQuestions []string `json:"follow_up_questions"`
	Insight           string   `json:"insight"`
	Encouragement     string   `json:"encouragement"`
	SuggestedAction   string   `json:"suggested_action"`
}

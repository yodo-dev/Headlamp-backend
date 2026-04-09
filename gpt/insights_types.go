package gpt

import "time"

// ─── Input context passed to GPT ─────────────────────────────────────────────

// InsightsContext is the aggregated behavioral data passed to GPT for analysis.
// All values are pre-computed by the service layer from DB queries.
type InsightsContext struct {
	ChildID        string `json:"child_id"`
	ChildFirstName string `json:"child_first_name"`
	ChildAge       int    `json:"child_age"`
	RangeDays      int    `json:"range_days"` // 1, 7, or 30

	Sessions    InsightSessionData    `json:"sessions"`
	Quizzes     InsightQuizData       `json:"quizzes"`
	Reflections InsightReflectionData `json:"reflections"`
}

// InsightSessionData aggregates app session signals.
type InsightSessionData struct {
	TotalSessions      int64             `json:"total_sessions"`
	TotalMinutes       float64           `json:"total_minutes"`
	AvgSessionMinutes  float64           `json:"avg_session_minutes"`
	ByApp              []InsightAppUsage `json:"by_app"`
	OverLimitIncidents int64             `json:"over_limit_incidents"`
	NightSessionCount  int64             `json:"night_session_count"`
}

// InsightAppUsage holds per-app usage during the window.
type InsightAppUsage struct {
	AppName           string  `json:"app_name"`
	Sessions          int64   `json:"sessions"`
	TotalMinutes      float64 `json:"total_minutes"`
	AvgSessionMinutes float64 `json:"avg_session_minutes"`
}

// InsightQuizData aggregates quiz performance signals.
type InsightQuizData struct {
	TotalAttempts int64   `json:"total_attempts"`
	PassCount     int64   `json:"pass_count"`
	FailCount     int64   `json:"fail_count"`
	AvgScore      float64 `json:"avg_score"`
	PassRate      float64 `json:"pass_rate"` // 0.0 – 1.0
}

// InsightReflectionData aggregates reflection engagement signals.
type InsightReflectionData struct {
	TotalDelivered  int64    `json:"total_delivered"`
	TotalResponded  int64    `json:"total_responded"`
	CompletionRate  float64  `json:"completion_rate"` // 0.0 – 1.0
	RecentResponses []string `json:"recent_responses"`
}

// ─── GPT response types ───────────────────────────────────────────────────────

// InsightsGPTResponse is the structured JSON that GPT returns after analysing
// the InsightsContext.  Each field maps directly to a section of the dashboard.
type InsightsGPTResponse struct {
	GuidanceCards      []GuidanceCard         `json:"guidance_cards"`
	EmotionalTrends    EmotionalTrends        `json:"emotional_trends"`
	BehavioralInsights []BehavioralInsight    `json:"behavioral_insights"`
	DigitalMaturity    DigitalMaturityProfile `json:"digital_maturity"`
}

// ─── Guidance ─────────────────────────────────────────────────────────────────

// GuidanceCard is a single actionable recommendation card.
type GuidanceCard struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Message    string   `json:"message"`
	Priority   string   `json:"priority"`   // low | medium | high
	Confidence float64  `json:"confidence"` // 0.0 – 1.0
	Tags       []string `json:"tags"`
}

// ─── Emotional Trends ─────────────────────────────────────────────────────────

// EmotionalTrends is the emotion time-series with a summary.
type EmotionalTrends struct {
	Series         []EmotionDataPoint `json:"series"`
	StabilityScore int                `json:"stability_score"` // 0 – 100
	Direction      string             `json:"direction"`       // improving | stable | declining
}

// EmotionDataPoint holds per-day emotion scores.
type EmotionDataPoint struct {
	Date        string  `json:"date"` // YYYY-MM-DD
	Confidence  float64 `json:"confidence"`
	Frustration float64 `json:"frustration"`
	Anxiety     float64 `json:"anxiety"`
	Empathy     float64 `json:"empathy"`
	Calm        float64 `json:"calm"`
}

// ─── Behavioral Insights ──────────────────────────────────────────────────────

// BehavioralInsight is a single parent-facing behavioral observation.
type BehavioralInsight struct {
	ID       string   `json:"id"`
	Headline string   `json:"headline"`
	Detail   string   `json:"detail"`
	Type     string   `json:"type"`     // strength | risk_flag | coaching_suggestion
	Evidence []string `json:"evidence"` // signal keys that drove this insight
}

// ─── Digital Maturity ─────────────────────────────────────────────────────────

// DigitalMaturityProfile is the per-dimension maturity assessment.
type DigitalMaturityProfile struct {
	OverallScore int                `json:"overall_score"` // 0 – 100
	Band         string             `json:"band"`          // emerging | developing | proficient
	Dimensions   MaturityDimensions `json:"dimensions"`
	NextSteps    []string           `json:"next_steps"`
	GeneratedAt  time.Time          `json:"generated_at"`
}

// MaturityDimensions holds scores for each maturity dimension.
type MaturityDimensions struct {
	SelfRegulation          int `json:"self_regulation"`
	SafetyAwareness         int `json:"safety_awareness"`
	RespectfulCommunication int `json:"respectful_communication"`
	CriticalThinking        int `json:"critical_thinking"`
	LearningConsistency     int `json:"learning_consistency"`
}

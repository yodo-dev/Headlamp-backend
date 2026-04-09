package gpt

// GPTResponse defines the structure for the AI's JSON output.
// This needs to be kept in sync with the system prompt.
type GPTResponse struct {
	QuestionText    string   `json:"question_text"`
	QuestionType    string   `json:"question_type"`
	Options         []string `json:"options"`
	Feedback        string   `json:"feedback"`
	PointsAwarded   float64  `json:"points_awarded"`
	CurrentScore    string   `json:"current_score"`
	IsFinalQuestion bool     `json:"is_final_question"`
	FinalSummary    string   `json:"final_summary"`
}

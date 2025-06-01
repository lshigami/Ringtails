package dto

import "time"

// QuestionResponseDTO is used for displaying question details to users.
type QuestionResponseDTO struct {
	ID          uint    `json:"id"`
	TestID      uint    `json:"test_id"`
	Title       string  `json:"title"`
	Prompt      string  `json:"prompt"`
	Type        string  `json:"type"`
	OrderInTest int     `json:"order_in_test"`
	ImageURL    *string `json:"image_url,omitempty"`
	GivenWord1  *string `json:"given_word1,omitempty"`
	GivenWord2  *string `json:"given_word2,omitempty"`
	MaxScore    float64 `json:"max_score"`
}

// TestResponseDTO is used for displaying full test details to users.
type TestResponseDTO struct {
	ID          uint                  `json:"id"`
	Title       string                `json:"title"`
	Description string                `json:"description,omitempty"`
	Questions   []QuestionResponseDTO `json:"questions,omitempty"`
	CreatedAt   time.Time             `json:"created_at"`
}

// TestSummaryDTO is used for listing tests available to users.
type TestSummaryDTO struct {
	ID            uint      `json:"id"`
	Title         string    `json:"title"`
	Description   string    `json:"description,omitempty"`
	QuestionCount int       `json:"question_count"`
	CreatedAt     time.Time `json:"created_at"`
}

// --- DTOs for Test Attempts (User submitting and viewing attempts) ---

// UserAnswerDTO represents a user's answer to a single question within a test submission.
type UserAnswerDTO struct {
	QuestionID uint   `json:"question_id" binding:"required"`
	UserAnswer string `json:"user_answer" binding:"required"`
}

// TestAttemptSubmitDTO is the request DTO for a user submitting all answers for a test.
type TestAttemptSubmitDTO struct {
	UserID  *uint           `json:"user_id"` // Temporary, for non-auth user identification
	Answers []UserAnswerDTO `json:"answers" binding:"required,dive"`
}

// AnswerResponseDTO is used for displaying individual answer details within a test attempt.
type AnswerResponseDTO struct {
	ID         uint                `json:"id"`
	QuestionID uint                `json:"question_id"`
	Question   QuestionResponseDTO `json:"question,omitempty"` // Contains full question details
	UserAnswer string              `json:"user_answer"`
	AIFeedback string              `json:"ai_feedback,omitempty"`
	AIScore    *float64            `json:"ai_score,omitempty"`
}

// TestAttemptDetailDTO is for displaying the full details of a specific test attempt.
type TestAttemptDetailDTO struct {
	ID          uint                `json:"id"`
	TestID      uint                `json:"test_id"`
	TestTitle   string              `json:"test_title,omitempty"`
	UserID      *uint               `json:"user_id,omitempty"`
	SubmittedAt time.Time           `json:"submitted_at"`
	TotalScore  *float64            `json:"total_score,omitempty"`
	Status      string              `json:"status"`
	Answers     []AnswerResponseDTO `json:"answers,omitempty"` // List of answers with their details
}

// TestAttemptSummaryDTO is for listing a user's attempts for a particular test.
type TestAttemptSummaryDTO struct {
	ID          uint      `json:"id"`
	TestID      uint      `json:"test_id"`
	UserID      *uint     `json:"user_id,omitempty"`
	SubmittedAt time.Time `json:"submitted_at"`
	TotalScore  *float64  `json:"total_score,omitempty"`
	Status      string    `json:"status"`
}

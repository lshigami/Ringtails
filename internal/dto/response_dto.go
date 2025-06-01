package dto

import "time"

type QuestionResponse struct {
	ID          uint      `json:"id"`
	TestID      *uint     `json:"test_id,omitempty"`
	Title       string    `json:"title"`
	Prompt      string    `json:"prompt"`
	Type        string    `json:"type"`
	OrderInTest int       `json:"order_in_test"`
	ImageURL    *string   `json:"image_url,omitempty"`
	GivenWord1  *string   `json:"given_word1,omitempty"`
	GivenWord2  *string   `json:"given_word2,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TestResponse struct {
	ID          uint               `json:"id"`
	Title       string             `json:"title"`
	Description string             `json:"description,omitempty"`
	Questions   []QuestionResponse `json:"questions,omitempty"` // Eager loaded questions
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

type AttemptResponse struct {
	ID          uint             `json:"id"`
	UserID      *uint            `json:"user_id,omitempty"`
	QuestionID  uint             `json:"question_id"`
	Question    QuestionResponse `json:"question,omitempty"`
	UserAnswer  string           `json:"user_answer"`
	AIFeedback  string           `json:"ai_feedback"`
	SubmittedAt time.Time        `json:"submitted_at"`
	CreatedAt   time.Time        `json:"created_at"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type SubmitFullTestResponse struct {
	TestID         uint              `json:"test_id"`
	UserID         *uint             `json:"user_id,omitempty"`
	SubmittedCount int               `json:"submitted_count"`
	Attempts       []AttemptResponse `json:"attempts"`
	Errors         []string          `json:"errors,omitempty"` // To report any partial failures
}

type QuestionAttemptHistoryDTO struct {
	QuestionID    uint             `json:"question_id"`
	QuestionTitle string           `json:"question_title"`
	QuestionType  string           `json:"question_type"`
	OrderInTest   int              `json:"order_in_test"`
	Prompt        string           `json:"prompt,omitempty"` // For email/essay
	ImageURL      *string          `json:"image_url,omitempty"`
	GivenWord1    *string          `json:"given_word1,omitempty"`
	GivenWord2    *string          `json:"given_word2,omitempty"`
	Attempts      []AttemptInfoDTO `json:"attempts"` // Chỉ chứa thông tin cần thiết của attempt
}

// AttemptInfoDTO is a simplified DTO for attempts within the history view.
type AttemptInfoDTO struct {
	AttemptID   uint      `json:"attempt_id"`
	UserAnswer  string    `json:"user_answer"`
	AIFeedback  string    `json:"ai_feedback,omitempty"` // Có thể là omitempty nếu không muốn load ngay
	SubmittedAt time.Time `json:"submitted_at"`
}

// TestAttemptHistoryResponseDTO is the response for fetching a user's attempt history for a specific test.
type TestAttemptHistoryResponseDTO struct {
	TestID           uint                        `json:"test_id"`
	TestTitle        string                      `json:"test_title"`
	UserID           *uint                       `json:"user_id,omitempty"`
	QuestionsHistory []QuestionAttemptHistoryDTO `json:"questions_history"`
}

// TestAttemptsResponse is the response for fetching all attempts for a specific test by a user.
type TestAttemptsResponse struct {
	TestID     uint              `json:"test_id"`
	TestTitle  string            `json:"test_title"`
	UserID     *uint             `json:"user_id,omitempty"`
	Attempts   []AttemptResponse `json:"attempts"`
	SubmittedAt *time.Time       `json:"submitted_at,omitempty"`
}

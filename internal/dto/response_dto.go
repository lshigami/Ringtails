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

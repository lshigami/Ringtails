package dto

// CreateQuestionRequest is for creating standalone questions or questions for a test later
type CreateQuestionRequest struct {
	TestID      *uint  `json:"test_id"` // Optional: associate with an existing test
	Title       string `json:"title" binding:"required"`
	Prompt      string `json:"prompt" binding:"required"` // Main text for email/essay, or general instruction for picture
	Type        string `json:"type" binding:"required,oneof=sentence_picture email_response opinion_essay"`
	OrderInTest int    `json:"order_in_test"` // 1-8, relevant if TestID is provided or part of CreateTestWithQuestionsRequest

	// For type="sentence_picture"
	ImageURL   *string `json:"image_url"`
	GivenWord1 *string `json:"given_word1"`
	GivenWord2 *string `json:"given_word2"`
}

// QuestionForTestRequest is used when creating questions as part of a new test
type QuestionForTestRequest struct {
	Title       string `json:"title" binding:"required"`
	Prompt      string `json:"prompt" binding:"required"`
	Type        string `json:"type" binding:"required,oneof=sentence_picture email_response opinion_essay"`
	OrderInTest int    `json:"order_in_test" binding:"required,min=1,max=8"`

	ImageURL   *string `json:"image_url"`   // Required if type="sentence_picture"
	GivenWord1 *string `json:"given_word1"` // Required if type="sentence_picture"
	GivenWord2 *string `json:"given_word2"` // Required if type="sentence_picture"
}

type CreateTestRequest struct {
	Title       string                   `json:"title" binding:"required"`
	Description string                   `json:"description"`
	Questions   []QuestionForTestRequest `json:"questions" binding:"omitempty,dive"` // Optional: create questions along with the test
}

type SubmitAttemptRequest struct {
	QuestionID uint   `json:"question_id" binding:"required"`
	UserAnswer string `json:"user_answer" binding:"required"`
}

package dto

// QuestionCreateDTO is used within TestCreateDTO for admin test creation.
type QuestionCreateDTO struct {
	Title       string  `json:"title" binding:"required"`
	Prompt      string  `json:"prompt" binding:"required"`
	Type        string  `json:"type" binding:"required,oneof=sentence_picture email_response opinion_essay"`
	OrderInTest int     `json:"order_in_test" binding:"required,min=1,max=8"`
	ImageURL    *string `json:"image_url"`
	GivenWord1  *string `json:"given_word1"`
	GivenWord2  *string `json:"given_word2"`
	MaxScore    float64 `json:"max_score" binding:"required,gt=0"`
}

// TestCreateDTO is for admin to create a new test with all its questions.
type TestCreateDTO struct {
	Title       string              `json:"title" binding:"required"`
	Description string              `json:"description,omitempty"`
	Questions   []QuestionCreateDTO `json:"questions" binding:"required,min=8,max=8,dive"`
}

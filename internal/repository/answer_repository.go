package repository

import (
	"github.com/lshigami/Ringtails/internal/model"
	"gorm.io/gorm"
)

type AnswerRepository interface {
	Update(answer *model.Answer) error
	// FindByTestAttemptIDAndQuestionID(testAttemptID uint, questionID uint) (*model.Answer, error) // Might be useful
}

type answerRepository struct {
	db *gorm.DB
}

func NewAnswerRepository(db *gorm.DB) AnswerRepository {
	return &answerRepository{db: db}
}

func (r *answerRepository) Update(answer *model.Answer) error {
	// Using Save to update all fields, including AIScore and AIFeedback
	return r.db.Save(answer).Error
}

// Example of a more specific find method if needed
// func (r *answerRepository) FindByTestAttemptIDAndQuestionID(testAttemptID uint, questionID uint) (*model.Answer, error) {
// 	var answer model.Answer
// 	err := r.db.Where("test_attempt_id = ? AND question_id = ?", testAttemptID, questionID).First(&answer).Error
// 	return &answer, err
// }

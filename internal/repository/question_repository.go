package repository

import (
	"github.com/lshigami/Ringtails/internal/model"
	"gorm.io/gorm"
)

type QuestionRepository interface {
	FindByID(id uint) (*model.Question, error)
	FindByTestID(testID uint) ([]model.Question, error)
	// Create, Update, Delete for individual Questions are generally handled
	// via the TestRepository due to the strong association (cascade on delete).
	// If standalone question management is needed, add methods here.
}

type questionRepository struct {
	db *gorm.DB
}

func NewQuestionRepository(db *gorm.DB) QuestionRepository {
	return &questionRepository{db: db}
}

func (r *questionRepository) FindByID(id uint) (*model.Question, error) {
	var q model.Question
	err := r.db.First(&q, id).Error
	return &q, err
}

func (r *questionRepository) FindByTestID(testID uint) ([]model.Question, error) {
	var questions []model.Question
	err := r.db.Where("test_id = ?", testID).Order("order_in_test ASC").Find(&questions).Error
	return questions, err
}

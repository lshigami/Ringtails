package repository

import (
	"github.com/lshigami/Ringtails/internal/model"
	"gorm.io/gorm"
)

type QuestionRepository interface {
	Create(question *model.Question) error
	FindByID(id uint) (*model.Question, error)
	FindAll() ([]model.Question, error)
	FindByTestID(testID uint) ([]model.Question, error)
	Update(question *model.Question) error
	Delete(id uint) error
}

type questionRepository struct {
	db *gorm.DB
}

func NewQuestionRepository(db *gorm.DB) QuestionRepository {
	return &questionRepository{db: db}
}

func (r *questionRepository) Create(question *model.Question) error {
	return r.db.Create(question).Error
}

func (r *questionRepository) FindByID(id uint) (*model.Question, error) {
	var question model.Question
	if err := r.db.First(&question, id).Error; err != nil {
		return nil, err
	}
	return &question, nil
}

func (r *questionRepository) FindAll() ([]model.Question, error) {
	var questions []model.Question
	if err := r.db.Order("created_at desc").Find(&questions).Error; err != nil {
		return nil, err
	}
	return questions, nil
}

func (r *questionRepository) FindByTestID(testID uint) ([]model.Question, error) {
	var questions []model.Question
	if err := r.db.Where("test_id = ?", testID).Order("order_in_test ASC").Find(&questions).Error; err != nil {
		return nil, err
	}
	return questions, nil
}

func (r *questionRepository) Update(question *model.Question) error {
	return r.db.Save(question).Error
}

func (r *questionRepository) Delete(id uint) error {
	return r.db.Delete(&model.Question{}, id).Error
}

package repository

import (
	"github.com/lshigami/Ringtails/internal/model"
	"gorm.io/gorm"
)

type AttemptRepository interface {
	Create(attempt *model.Attempt) error
	FindByID(id uint) (*model.Attempt, error)
	FindAllWithQuestions() ([]model.Attempt, error) // Eager load questions
	Update(attempt *model.Attempt) error
}

type attemptRepository struct {
	db *gorm.DB
}

func NewAttemptRepository(db *gorm.DB) AttemptRepository {
	return &attemptRepository{db: db}
}

func (r *attemptRepository) Create(attempt *model.Attempt) error {
	return r.db.Create(attempt).Error
}

func (r *attemptRepository) FindByID(id uint) (*model.Attempt, error) {
	var attempt model.Attempt
	if err := r.db.Preload("Question").First(&attempt, id).Error; err != nil {
		return nil, err
	}
	return &attempt, nil
}

func (r *attemptRepository) FindAllWithQuestions() ([]model.Attempt, error) {
	var attempts []model.Attempt
	// Order by SubmittedAt descending to get the latest attempts first
	if err := r.db.Preload("Question").Order("submitted_at desc").Find(&attempts).Error; err != nil {
		return nil, err
	}
	return attempts, nil
}

func (r *attemptRepository) Update(attempt *model.Attempt) error {
	return r.db.Save(attempt).Error
}

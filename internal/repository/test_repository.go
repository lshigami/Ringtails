package repository

import (
	"github.com/lshigami/Ringtails/internal/model"
	"gorm.io/gorm"
)

type TestRepository interface {
	Create(test *model.Test) error
	FindByID(id uint) (*model.Test, error)
	FindByIDWithQuestions(id uint) (*model.Test, error)
	FindAllWithQuestionCount() ([]struct {
		model.Test
		QuestionCount int
	}, error)
	// Update(test *model.Test) error // Add if admin needs to update test metadata
	// Delete(id uint) error         // Add if admin needs to delete tests
}

type testRepository struct {
	db *gorm.DB
}

func NewTestRepository(db *gorm.DB) TestRepository {
	return &testRepository{db: db}
}

func (r *testRepository) Create(test *model.Test) error {
	// GORM's Create with associations will handle creating questions if test.Questions is populated
	// and Question model has TestID foreign key, and Test model has Questions []Question `gorm:"foreignKey:TestID"`
	return r.db.Create(test).Error
}

func (r *testRepository) FindByID(id uint) (*model.Test, error) {
	var test model.Test
	err := r.db.First(&test, id).Error
	return &test, err
}

func (r *testRepository) FindByIDWithQuestions(id uint) (*model.Test, error) {
	var test model.Test
	err := r.db.Preload("Questions", func(db *gorm.DB) *gorm.DB {
		return db.Order("questions.order_in_test ASC")
	}).First(&test, id).Error
	return &test, err
}

func (r *testRepository) FindAllWithQuestionCount() ([]struct {
	model.Test
	QuestionCount int
}, error) {
	var results []struct {
		model.Test
		QuestionCount int
	}
	err := r.db.Model(&model.Test{}).
		Select("tests.*, (SELECT COUNT(*) FROM questions WHERE questions.test_id = tests.id AND questions.deleted_at IS NULL) as question_count").
		Order("tests.created_at DESC").
		Where("tests.deleted_at IS NULL"). // Only select non-deleted tests
		Scan(&results).Error
	return results, err
}

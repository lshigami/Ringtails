package repository

import (
	"github.com/lshigami/Ringtails/internal/model"
	"gorm.io/gorm"
)

type TestRepository interface {
	Create(test *model.Test) error
	FindByID(id uint) (*model.Test, error)
	FindByIDWithQuestions(id uint) (*model.Test, error)
	FindAll() ([]model.Test, error)
	FindAllWithQuestions() ([]model.Test, error)
	Update(test *model.Test) error
	Delete(id uint) error
}

type testRepository struct {
	db *gorm.DB
}

func NewTestRepository(db *gorm.DB) TestRepository {
	return &testRepository{db: db}
}

func (r *testRepository) Create(test *model.Test) error {
	for i := range test.Questions {
		test.Questions[i].TestID = &test.ID
	}
	return r.db.Create(test).Error
}

func (r *testRepository) FindByID(id uint) (*model.Test, error) {
	var test model.Test
	if err := r.db.First(&test, id).Error; err != nil {
		return nil, err
	}
	return &test, nil
}

func (r *testRepository) FindByIDWithQuestions(id uint) (*model.Test, error) {
	var test model.Test
	if err := r.db.Preload("Questions", func(db *gorm.DB) *gorm.DB {
		return db.Order("questions.order_in_test ASC")
	}).First(&test, id).Error; err != nil {
		return nil, err
	}
	return &test, nil
}

func (r *testRepository) FindAll() ([]model.Test, error) {
	var tests []model.Test
	if err := r.db.Order("created_at desc").Find(&tests).Error; err != nil {
		return nil, err
	}
	return tests, nil
}

func (r *testRepository) FindAllWithQuestions() ([]model.Test, error) {
	var tests []model.Test
	if err := r.db.Preload("Questions", func(db *gorm.DB) *gorm.DB {
		return db.Order("questions.order_in_test ASC")
	}).Order("created_at desc").Find(&tests).Error; err != nil {
		return nil, err
	}
	return tests, nil
}

func (r *testRepository) Update(test *model.Test) error {
	return r.db.Session(&gorm.Session{FullSaveAssociations: true}).Updates(test).Error

}

func (r *testRepository) Delete(id uint) error {
	return r.db.Delete(&model.Test{}, id).Error
}

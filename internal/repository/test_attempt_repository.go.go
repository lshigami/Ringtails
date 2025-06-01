package repository

import (
	"github.com/lshigami/Ringtails/internal/model"
	"gorm.io/gorm"
)

type TestAttemptRepository interface {
	Create(attempt *model.TestAttempt) error
	Update(attempt *model.TestAttempt) error
	FindByID(id uint) (*model.TestAttempt, error)
	FindByIDWithDetails(id uint) (*model.TestAttempt, error)
	FindAllByTestAndUser(testID uint, userID *uint) ([]model.TestAttempt, error)
}

type testAttemptRepository struct {
	db *gorm.DB
}

func NewTestAttemptRepository(db *gorm.DB) TestAttemptRepository {
	return &testAttemptRepository{db: db}
}

func (r *testAttemptRepository) Create(attempt *model.TestAttempt) error {
	// GORM will create associated Answers if attempt.Answers is populated
	// and Answer model has TestAttemptID, and TestAttempt has Answers []Answer `gorm:"foreignKey:TestAttemptID"`
	return r.db.Create(attempt).Error
}

func (r *testAttemptRepository) Update(attempt *model.TestAttempt) error {
	// Save will update the TestAttempt and its associations if they are loaded and modified.
	// If only updating TestAttempt fields (like TotalScore, Status), a Model().Updates() might be more precise.
	// However, Save is generally fine if you ensure associations are handled correctly at the service layer.
	return r.db.Save(attempt).Error
}

func (r *testAttemptRepository) FindByID(id uint) (*model.TestAttempt, error) {
	var attempt model.TestAttempt
	err := r.db.First(&attempt, id).Error
	return &attempt, err
}

func (r *testAttemptRepository) FindByIDWithDetails(id uint) (*model.TestAttempt, error) {
	var attempt model.TestAttempt
	err := r.db.
		Preload("Test").             // Preload the Test details
		Preload("Answers.Question"). // Preload Answers and their associated Questions
		First(&attempt, id).Error
	return &attempt, err
}

func (r *testAttemptRepository) FindAllByTestAndUser(testID uint, userID *uint) ([]model.TestAttempt, error) {
	var attempts []model.TestAttempt
	query := r.db.Where("test_id = ?", testID)
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	// Optionally preload Test info for each attempt summary, or just rely on TestID
	// query = query.Preload("Test")
	err := query.Order("submitted_at DESC").Find(&attempts).Error
	return attempts, err
}

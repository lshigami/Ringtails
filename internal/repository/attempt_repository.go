package repository

import (
	"github.com/lshigami/Ringtails/internal/model"
	"gorm.io/gorm"
)

type AttemptRepository interface {
	Create(attempt *model.Attempt) error
	FindByID(id uint) (*model.Attempt, error)
	FindAllWithQuestions(userID *uint, testID *uint) ([]model.Attempt, error)
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
	// Preload Question, and ensure UserID is included if needed in future
	if err := r.db.Preload("Question").First(&attempt, id).Error; err != nil {
		return nil, err
	}
	return &attempt, nil
}

func (r *attemptRepository) FindAllWithQuestions(userID *uint, testID *uint) ([]model.Attempt, error) {
	var attempts []model.Attempt
	// Bắt đầu query với Preload và Order
	query := r.db.Preload("Question").Order("attempts.submitted_at desc") // Sử dụng "attempts.submitted_at" để rõ ràng hơn khi join

	if userID != nil {
		query = query.Where("attempts.user_id = ?", *userID)
	}

	if testID != nil {
		// Chúng ta cần join với bảng questions để lọc theo test_id
		// Hoặc nếu Question model đã có TestID, chúng ta có thể lọc trên Question model đã preload
		// Cách 1: Join (phức tạp hơn một chút nếu Question chưa được join)
		// query = query.Joins("JOIN questions ON questions.id = attempts.question_id AND questions.test_id = ?", *testID)

		// Cách 2: Sử dụng Subquery hoặc lọc trên ID câu hỏi thuộc Test (đơn giản hơn nếu có danh sách ID câu hỏi)
		// Giả sử chúng ta muốn lọc các attempts mà Question của nó thuộc về testID.
		// Chúng ta cần lấy danh sách các question_id thuộc về testID đó trước.
		// Tuy nhiên, GORM cho phép Preload với conditions.
		// Cách hiệu quả nhất là lọc trực tiếp các attempts có question_id nằm trong tập các question_id của test đó.
		// Hoặc, nếu cấu trúc cho phép, lọc trên trường `Question.TestID` sau khi preload.
		// Nếu `attempts` có `Question` và `Question` có `TestID`, chúng ta có thể lọc sau khi lấy dữ liệu, nhưng không hiệu quả.
		// Tốt nhất là lọc ở DB.
		//
		// Cách đơn giản nhất để làm ở DB với GORM khi có quan hệ:
		// Lọc các attempts mà `question_id` của nó nằm trong danh sách các `id` của `questions` có `test_id` = *testID
		query = query.Where("attempts.question_id IN (SELECT id FROM questions WHERE test_id = ?)", *testID)
	}

	if err := query.Find(&attempts).Error; err != nil {
		return nil, err
	}
	return attempts, nil
}

func (r *attemptRepository) Update(attempt *model.Attempt) error {
	return r.db.Save(attempt).Error
}

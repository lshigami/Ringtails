package service

import (
	"fmt"

	"github.com/jinzhu/copier"
	"github.com/lshigami/Ringtails/internal/dto"
	"github.com/lshigami/Ringtails/internal/model"
	"github.com/lshigami/Ringtails/internal/repository"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type TestService interface {
	CreateTest(req dto.CreateTestRequest) (*dto.TestResponse, error)
	GetTestWithQuestions(id uint) (*dto.TestResponse, error)
	GetAllTests(withQuestions bool) ([]dto.TestResponse, error)
	UpdateTest(id uint, req dto.CreateTestRequest) (*dto.TestResponse, error) // For title/desc, not questions here
	DeleteTest(id uint) error
	AddQuestionToTest(testID uint, qReq dto.CreateQuestionRequest) (*dto.QuestionResponse, error)
}

type testService struct {
	testRepo     repository.TestRepository
	questionRepo repository.QuestionRepository
	db           *gorm.DB // For transactions
}

func NewTestService(testRepo repository.TestRepository, questionRepo repository.QuestionRepository, db *gorm.DB) TestService {
	return &testService{testRepo: testRepo, questionRepo: questionRepo, db: db}
}

func (s *testService) CreateTest(req dto.CreateTestRequest) (*dto.TestResponse, error) {
	test := model.Test{
		Title:       req.Title,
		Description: req.Description,
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Create the test
		if err := tx.Create(&test).Error; err != nil {
			return err
		}

		// If questions are provided, create them and associate
		if len(req.Questions) > 0 {
			for _, qReq := range req.Questions {
				if qReq.Type == "sentence_picture" && (qReq.ImageURL == nil || qReq.GivenWord1 == nil || qReq.GivenWord2 == nil) {
					return fmt.Errorf("question (OrderInTest: %d) of type 'sentence_picture' requires ImageURL, GivenWord1, and GivenWord2", qReq.OrderInTest)
				}
				question := model.Question{
					TestID:      &test.ID, // Associate with the newly created test
					Title:       qReq.Title,
					Prompt:      qReq.Prompt,
					Type:        qReq.Type,
					OrderInTest: qReq.OrderInTest,
					ImageURL:    qReq.ImageURL,
					GivenWord1:  qReq.GivenWord1,
					GivenWord2:  qReq.GivenWord2,
				}
				if err := tx.Create(&question).Error; err != nil {
					return fmt.Errorf("failed to create question (OrderInTest %d) for test %s: %w", qReq.OrderInTest, test.Title, err)
				}
				test.Questions = append(test.Questions, question) // For the response DTO
			}
		}
		return nil
	})

	if err != nil {
		log.Error().Err(err).Msg("Failed to create test with questions in transaction")
		return nil, err
	}

	var resp dto.TestResponse
	copier.Copy(&resp, &test)
	// Manually copy questions if copier doesn't handle it perfectly after transaction
	if len(req.Questions) > 0 && len(resp.Questions) == 0 && len(test.Questions) > 0 {
		copier.Copy(&resp.Questions, &test.Questions)
	}
	return &resp, nil
}

func (s *testService) GetTestWithQuestions(id uint) (*dto.TestResponse, error) {
	test, err := s.testRepo.FindByIDWithQuestions(id)
	if err != nil {
		return nil, err
	}
	var resp dto.TestResponse
	copier.Copy(&resp, test)
	return &resp, nil
}

func (s *testService) GetAllTests(withQuestions bool) ([]dto.TestResponse, error) {
	var tests []model.Test
	var err error
	if withQuestions {
		tests, err = s.testRepo.FindAllWithQuestions()
	} else {
		tests, err = s.testRepo.FindAll()
	}
	if err != nil {
		return nil, err
	}
	var resp []dto.TestResponse
	copier.Copy(&resp, &tests)
	return resp, nil
}

func (s *testService) UpdateTest(id uint, req dto.CreateTestRequest) (*dto.TestResponse, error) {
	test, err := s.testRepo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("test not found with ID %d", id)
	}
	test.Title = req.Title
	test.Description = req.Description
	// Note: This update does not handle updating associated questions.
	// For updating questions, use QuestionService or a dedicated TestQuestion management method.
	if err := s.testRepo.Update(test); err != nil {
		return nil, err
	}
	var resp dto.TestResponse
	copier.Copy(&resp, test)
	return &resp, nil
}

func (s *testService) DeleteTest(id uint) error {
	// Consider what to do with associated questions:
	// 1. Delete them (cascade delete in DB or manually here in a transaction)
	// 2. Unlink them (set TestID to NULL)
	// For now, just delete the test. DB constraints or GORM hooks could handle cascades.
	// To be safe, explicitly delete questions in a transaction:
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Delete associated questions first
		if err := tx.Where("test_id = ?", id).Delete(&model.Question{}).Error; err != nil {
			return err
		}
		// Then delete the test
		if err := tx.Delete(&model.Test{}, id).Error; err != nil {
			return err
		}
		return nil
	})
}

func (s *testService) AddQuestionToTest(testID uint, qReq dto.CreateQuestionRequest) (*dto.QuestionResponse, error) {
	_, err := s.testRepo.FindByID(testID)
	if err != nil {
		return nil, fmt.Errorf("test with ID %d not found", testID)
	}

	if qReq.Type == "sentence_picture" && (qReq.ImageURL == nil || qReq.GivenWord1 == nil || qReq.GivenWord2 == nil) {
		return nil, fmt.Errorf("question of type 'sentence_picture' requires ImageURL, GivenWord1, and GivenWord2")
	}

	question := model.Question{
		TestID:      &testID,
		Title:       qReq.Title,
		Prompt:      qReq.Prompt,
		Type:        qReq.Type,
		OrderInTest: qReq.OrderInTest,
		ImageURL:    qReq.ImageURL,
		GivenWord1:  qReq.GivenWord1,
		GivenWord2:  qReq.GivenWord2,
	}

	if err := s.questionRepo.Create(&question); err != nil {
		log.Error().Err(err).Msg("Failed to add question to test")
		return nil, err
	}

	var resp dto.QuestionResponse
	copier.Copy(&resp, &question)
	return &resp, nil
}

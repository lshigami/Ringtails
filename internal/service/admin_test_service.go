package service

import (
	"fmt"

	"github.com/jinzhu/copier"
	"github.com/lshigami/Ringtails/internal/dto" // Corrected DTO path
	"github.com/lshigami/Ringtails/internal/model"
	"github.com/lshigami/Ringtails/internal/repository"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type AdminTestService interface {
	CreateTest(req dto.TestCreateDTO) (*dto.TestResponseDTO, error)
}

type adminTestService struct {
	testRepo repository.TestRepository
	db       *gorm.DB // db instance for potential direct use or complex transactions
}

func NewAdminTestService(testRepo repository.TestRepository, db *gorm.DB) AdminTestService {
	return &adminTestService{testRepo: testRepo, db: db}
}

func (s *adminTestService) CreateTest(req dto.TestCreateDTO) (*dto.TestResponseDTO, error) {
	// Validate uniqueness of OrderInTest and other question-specific rules
	if len(req.Questions) != 8 {
		return nil, fmt.Errorf("a test must have exactly 8 questions, received %d", len(req.Questions))
	}
	orderMap := make(map[int]bool)
	for _, qDto := range req.Questions {
		if _, exists := orderMap[qDto.OrderInTest]; exists {
			return nil, fmt.Errorf("duplicate OrderInTest %d found in questions", qDto.OrderInTest)
		}
		orderMap[qDto.OrderInTest] = true
		if qDto.OrderInTest < 1 || qDto.OrderInTest > 8 {
			return nil, fmt.Errorf("OrderInTest must be between 1 and 8, got %d for question '%s'", qDto.OrderInTest, qDto.Title)
		}

		if qDto.Type == "sentence_picture" && (qDto.ImageURL == nil || *qDto.ImageURL == "" || qDto.GivenWord1 == nil || *qDto.GivenWord1 == "" || qDto.GivenWord2 == nil || *qDto.GivenWord2 == "") {
			return nil, fmt.Errorf("question '%s' (Order: %d) of type 'sentence_picture' requires ImageURL, GivenWord1, and GivenWord2 to be non-empty", qDto.Title, qDto.OrderInTest)
		}
		if qDto.MaxScore <= 0 {
			return nil, fmt.Errorf("MaxScore for question '%s' (Order: %d) must be greater than 0", qDto.Title, qDto.OrderInTest)
		}
	}

	var testModel model.Test
	// Copy basic Test info
	testModel.Title = req.Title
	testModel.Description = req.Description

	// Manually build questions to ensure correct association and GORM behavior
	for _, qDto := range req.Questions {
		var questionModel model.Question
		copier.Copy(&questionModel, &qDto)
		// TestID will be set by GORM automatically when creating Test with associations
		testModel.Questions = append(testModel.Questions, questionModel)
	}

	if err := s.testRepo.Create(&testModel); err != nil {
		log.Error().Err(err).Msg("Failed to create test in database")
		return nil, fmt.Errorf("database error creating test: %w", err)
	}

	// The testModel now has IDs populated, including for nested Questions.
	// We can use copier to map it to TestResponseDTO.
	// FindByIDWithQuestions is good to ensure all associations are correctly loaded for the response.
	createdTestWithDetails, err := s.testRepo.FindByIDWithQuestions(testModel.ID)
	if err != nil {
		log.Error().Err(err).Uint("testID", testModel.ID).Msg("Failed to retrieve newly created test with questions for response")
		// Fallback: Use the model we have, though questions might not be fully "reloaded" in the same way.
		// However, GORM populates IDs on create, so basic info is there.
		var fallbackResp dto.TestResponseDTO
		copier.Copy(&fallbackResp, &testModel) // testModel has IDs from the Create operation
		return &fallbackResp, nil
	}

	var resp dto.TestResponseDTO
	if err := copier.Copy(&resp, createdTestWithDetails); err != nil {
		log.Error().Err(err).Msg("Failed to copy created Test model to TestResponseDTO")
		return nil, fmt.Errorf("error preparing response data: %w", err)
	}
	return &resp, nil
}

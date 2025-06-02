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

type AdminTestService interface {
	CreateTest(req dto.TestCreateDTO) (*dto.TestResponseDTO, error)
}

type adminTestService struct {
	testRepo repository.TestRepository
	db       *gorm.DB
}

func NewAdminTestService(testRepo repository.TestRepository, db *gorm.DB) AdminTestService {
	return &adminTestService{testRepo: testRepo, db: db}
}

func (s *adminTestService) CreateTest(req dto.TestCreateDTO) (*dto.TestResponseDTO, error) {
	if len(req.Questions) != 8 {
		return nil, fmt.Errorf("a test must have exactly 8 questions, received %d", len(req.Questions))
	}

	orderMap := make(map[int]bool)
	var questionsToCreateModel []model.Question

	for _, qDto := range req.Questions {
		if _, exists := orderMap[qDto.OrderInTest]; exists {
			return nil, fmt.Errorf("duplicate OrderInTest %d found in questions", qDto.OrderInTest)
		}
		orderMap[qDto.OrderInTest] = true

		if qDto.OrderInTest < 1 || qDto.OrderInTest > 8 {
			return nil, fmt.Errorf("OrderInTest must be between 1 and 8, got %d for question '%s'", qDto.OrderInTest, qDto.Title)
		}

		// Validate MaxScore based on OrderInTest and Type
		expectedMaxScore := 0.0
		switch {
		case qDto.OrderInTest >= 1 && qDto.OrderInTest <= 5: // Part 1: Q1-5
			if qDto.Type != "sentence_picture" {
				return nil, fmt.Errorf("question %d (Order: %d) should be type 'sentence_picture'", qDto.OrderInTest, qDto.OrderInTest)
			}
			expectedMaxScore = 3.0
			if qDto.ImageURL == nil || *qDto.ImageURL == "" || qDto.GivenWord1 == nil || *qDto.GivenWord1 == "" || qDto.GivenWord2 == nil || *qDto.GivenWord2 == "" {
				return nil, fmt.Errorf("question '%s' (Order: %d) of type 'sentence_picture' requires ImageURL, GivenWord1, and GivenWord2 to be non-empty", qDto.Title, qDto.OrderInTest)
			}
		case qDto.OrderInTest >= 6 && qDto.OrderInTest <= 7: // Part 2: Q6-7
			if qDto.Type != "email_response" {
				return nil, fmt.Errorf("question %d (Order: %d) should be type 'email_response'", qDto.OrderInTest, qDto.OrderInTest)
			}
			expectedMaxScore = 4.0
		case qDto.OrderInTest == 8: // Part 3: Q8
			if qDto.Type != "opinion_essay" {
				return nil, fmt.Errorf("question %d (Order: %d) should be type 'opinion_essay'", qDto.OrderInTest, qDto.OrderInTest)
			}
			expectedMaxScore = 5.0
		default:
			return nil, fmt.Errorf("invalid OrderInTest: %d", qDto.OrderInTest)
		}

		if qDto.MaxScore != expectedMaxScore {
			return nil, fmt.Errorf("MaxScore for question '%s' (Order: %d, Type: %s) should be %.1f, but got %.1f", qDto.Title, qDto.OrderInTest, qDto.Type, expectedMaxScore, qDto.MaxScore)
		}

		var questionModel model.Question
		copier.Copy(&questionModel, &qDto)
		questionsToCreateModel = append(questionsToCreateModel, questionModel)
	}

	testModel := model.Test{
		Title:       req.Title,
		Description: req.Description,
		Questions:   questionsToCreateModel,
	}

	if err := s.testRepo.Create(&testModel); err != nil {
		log.Error().Err(err).Msg("Failed to create test in database")
		return nil, fmt.Errorf("database error creating test: %w", err)
	}

	createdTestWithDetails, err := s.testRepo.FindByIDWithQuestions(testModel.ID)
	if err != nil {
		log.Error().Err(err).Uint("testID", testModel.ID).Msg("Failed to retrieve newly created test with questions for response")
		var fallbackResp dto.TestResponseDTO
		copier.Copy(&fallbackResp, &testModel)
		return &fallbackResp, nil
	}

	var resp dto.TestResponseDTO
	if err := copier.Copy(&resp, createdTestWithDetails); err != nil {
		log.Error().Err(err).Msg("Failed to copy created Test model to TestResponseDTO")
		return nil, fmt.Errorf("error preparing response data: %w", err)
	}
	return &resp, nil
}

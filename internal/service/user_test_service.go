package service

import (
	"fmt"

	"github.com/jinzhu/copier"
	"github.com/lshigami/Ringtails/internal/dto" // Corrected DTO path
	"github.com/lshigami/Ringtails/internal/repository"
	"github.com/rs/zerolog/log"
)

type UserTestService interface {
	GetAllTests() ([]dto.TestSummaryDTO, error)
	GetTestDetails(testID uint) (*dto.TestResponseDTO, error)
}

type userTestService struct {
	testRepo repository.TestRepository
}

func NewUserTestService(testRepo repository.TestRepository) UserTestService {
	return &userTestService{testRepo: testRepo}
}

func (s *userTestService) GetAllTests() ([]dto.TestSummaryDTO, error) {
	testsWithCount, err := s.testRepo.FindAllWithQuestionCount()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get all tests with question count from repository")
		return nil, fmt.Errorf("error fetching tests: %w", err)
	}

	var dtos []dto.TestSummaryDTO
	for _, twc := range testsWithCount {
		dtos = append(dtos, dto.TestSummaryDTO{
			ID:            twc.Test.ID,
			Title:         twc.Test.Title,
			Description:   twc.Test.Description,
			QuestionCount: twc.QuestionCount,
			CreatedAt:     twc.Test.CreatedAt,
		})
	}
	return dtos, nil
}

func (s *userTestService) GetTestDetails(testID uint) (*dto.TestResponseDTO, error) {
	test, err := s.testRepo.FindByIDWithQuestions(testID)
	if err != nil {
		log.Error().Err(err).Uint("testID", testID).Msg("Failed to get test details from repository")
		// Consider returning a more specific "not found" error if gorm.ErrRecordNotFound
		return nil, fmt.Errorf("test not found with ID %d: %w", testID, err)
	}

	var resp dto.TestResponseDTO
	if err := copier.Copy(&resp, test); err != nil {
		log.Error().Err(err).Msg("Failed to copy Test model to TestResponseDTO")
		return nil, fmt.Errorf("error preparing test details response: %w", err)
	}
	return &resp, nil
}

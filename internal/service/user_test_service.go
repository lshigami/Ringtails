package service

import (
	"fmt"

	"github.com/jinzhu/copier"
	"github.com/lshigami/Ringtails/internal/dto" // Corrected DTO path
	"github.com/lshigami/Ringtails/internal/repository"
	"github.com/rs/zerolog/log"
)

type UserTestService interface {
	GetAllTests(userID *uint) ([]dto.TestSummaryDTO, error)
	GetTestDetails(testID uint) (*dto.TestResponseDTO, error)
}

type userTestService struct {
	testRepo        repository.TestRepository
	testAttemptRepo repository.TestAttemptRepository
	scoreConverter  ScoreConverterService
}

func NewUserTestService(
	testRepo repository.TestRepository,
	testAttemptRepo repository.TestAttemptRepository,
	scoreConverter ScoreConverterService, // Inject
) UserTestService {
	return &userTestService{
		testRepo:        testRepo,
		testAttemptRepo: testAttemptRepo,
		scoreConverter:  scoreConverter, // Gán
	}
}

func (s *userTestService) GetAllTests(requestingUserID *uint) ([]dto.TestSummaryDTO, error) {
	testsWithCount, err := s.testRepo.FindAllWithQuestionCount()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get all tests with question count from repository")
		return nil, fmt.Errorf("error fetching tests: %w", err)
	}

	var dtos []dto.TestSummaryDTO
	for _, twc := range testsWithCount {
		summary := dto.TestSummaryDTO{
			ID:            twc.Test.ID,
			Title:         twc.Test.Title,
			Description:   twc.Test.Description,
			QuestionCount: twc.QuestionCount,
			CreatedAt:     twc.Test.CreatedAt,
		}

		if requestingUserID != nil {
			latestAttempt, errLatest := s.testAttemptRepo.FindLatestByTestAndUser(twc.Test.ID, *requestingUserID)
			if errLatest != nil {
				log.Error().Err(errLatest).Uint("userID", *requestingUserID).Uint("testID", twc.Test.ID).Msg("Error fetching latest test attempt for summary")
			} else if latestAttempt != nil {
				hasAttempted := true
				summary.HasAttemptedByUser = &hasAttempted
				summary.LastAttemptStatus = &latestAttempt.Status
				if latestAttempt.TotalScore != nil {
					summary.LastAttemptRawScore = latestAttempt.TotalScore
					scaledScore, errScale := s.scoreConverter.ConvertToScaledScore(*latestAttempt.TotalScore)
					if errScale != nil {
						log.Warn().Err(errScale).Float64("rawScore", *latestAttempt.TotalScore).Msg("Failed to scale score for test summary")
					} else {
						summary.LastAttemptScaledScore = &scaledScore
					}
				}
			} else {
				hasAttempted := false
				summary.HasAttemptedByUser = &hasAttempted
			}
		}
		dtos = append(dtos, summary)
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

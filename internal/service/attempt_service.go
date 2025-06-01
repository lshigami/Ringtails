package service

import (
	"time"

	"github.com/jinzhu/copier"
	"github.com/lshigami/Ringtails/internal/dto"
	"github.com/lshigami/Ringtails/internal/model"
	"github.com/lshigami/Ringtails/internal/repository"
	"github.com/rs/zerolog/log"
)

type AttemptService interface {
	SubmitAttempt(req dto.SubmitAttemptRequest) (*dto.AttemptResponse, error)
	GetAttempt(id uint) (*dto.AttemptResponse, error)
	GetAllAttempts() ([]dto.AttemptResponse, error)
}

type attemptService struct {
	attemptRepo  repository.AttemptRepository
	questionRepo repository.QuestionRepository
	geminiSvc    GeminiService // Interface for GeminiService
}

func NewAttemptService(
	attemptRepo repository.AttemptRepository,
	questionRepo repository.QuestionRepository,
	geminiSvc GeminiService,
) AttemptService {
	return &attemptService{
		attemptRepo:  attemptRepo,
		questionRepo: questionRepo,
		geminiSvc:    geminiSvc,
	}
}

func (s *attemptService) SubmitAttempt(req dto.SubmitAttemptRequest) (*dto.AttemptResponse, error) {
	question, err := s.questionRepo.FindByID(req.QuestionID) // This now fetches the question with new fields
	if err != nil {
		log.Error().Err(err).Uint("questionId", req.QuestionID).Msg("Failed to find question for submission")
		return nil, err
	}

	attempt := model.Attempt{
		QuestionID:  req.QuestionID,
		UserAnswer:  req.UserAnswer,
		SubmittedAt: time.Now(),
	}

	// Get AI Feedback using the full question object
	aiFeedback, err := s.geminiSvc.GetFeedbackForAttempt(question, req.UserAnswer)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get AI feedback")
		attempt.AIFeedback = "Error retrieving AI feedback: " + err.Error()
	} else {
		attempt.AIFeedback = aiFeedback
	}

	if err := s.attemptRepo.Create(&attempt); err != nil {
		log.Error().Err(err).Msg("Failed to create attempt in DB")
		return nil, err
	}

	fullAttempt, err := s.attemptRepo.FindByID(attempt.ID) // This reloads with Question preloaded
	if err != nil {
		log.Error().Err(err).Uint("attemptId", attempt.ID).Msg("Failed to reload attempt after creation")
		var resp dto.AttemptResponse
		copier.Copy(&resp, &attempt) // Fallback
		// Try to copy question details manually from initial fetch if preload failed
		copier.Copy(&resp.Question, question)
		return &resp, nil
	}

	var resp dto.AttemptResponse
	copier.Copy(&resp, fullAttempt)
	// Ensure Question within AttemptResponse is populated
	// copier should handle this if fullAttempt.Question is populated by Preload.
	// If not, you might need: copier.Copy(&resp.Question, &fullAttempt.Question)
	return &resp, nil
}

func (s *attemptService) GetAttempt(id uint) (*dto.AttemptResponse, error) {
	attempt, err := s.attemptRepo.FindByID(id)
	if err != nil {
		return nil, err
	}
	var resp dto.AttemptResponse
	copier.Copy(&resp, attempt)
	return &resp, nil
}

func (s *attemptService) GetAllAttempts() ([]dto.AttemptResponse, error) {
	attempts, err := s.attemptRepo.FindAllWithQuestions()
	if err != nil {
		return nil, err
	}
	var resp []dto.AttemptResponse
	copier.Copy(&resp, &attempts)
	return resp, nil
}

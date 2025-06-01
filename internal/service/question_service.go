package service

import (
	"fmt"

	"github.com/jinzhu/copier"
	"github.com/lshigami/Ringtails/internal/dto"
	"github.com/lshigami/Ringtails/internal/model"
	"github.com/lshigami/Ringtails/internal/repository"
	"github.com/rs/zerolog/log"
)

type QuestionService interface {
	CreateQuestion(req dto.CreateQuestionRequest) (*dto.QuestionResponse, error)
	GetQuestion(id uint) (*dto.QuestionResponse, error)
	GetAllQuestions(testID *uint) ([]dto.QuestionResponse, error) // Pass TestID to filter
	UpdateQuestion(id uint, req dto.CreateQuestionRequest) (*dto.QuestionResponse, error)
	DeleteQuestion(id uint) error
}

type questionService struct {
	repo     repository.QuestionRepository
	testRepo repository.TestRepository // To validate TestID if provided
}

func NewQuestionService(repo repository.QuestionRepository, testRepo repository.TestRepository) QuestionService {
	return &questionService{repo: repo, testRepo: testRepo}
}

func (s *questionService) CreateQuestion(req dto.CreateQuestionRequest) (*dto.QuestionResponse, error) {
	if req.TestID != nil {
		_, err := s.testRepo.FindByID(*req.TestID)
		if err != nil {
			log.Warn().Err(err).Uint("testID", *req.TestID).Msg("Invalid TestID provided for question creation")
			return nil, fmt.Errorf("invalid TestID: %v", *req.TestID)
		}
	}
	if req.Type == "sentence_picture" && (req.ImageURL == nil || req.GivenWord1 == nil || req.GivenWord2 == nil) {
		return nil, fmt.Errorf("ImageURL, GivenWord1, and GivenWord2 are required for sentence_picture type")
	}

	question := model.Question{}
	copier.Copy(&question, &req) // TestID, ImageURL, GivenWord1, GivenWord2 should copy if names match

	if err := s.repo.Create(&question); err != nil {
		log.Error().Err(err).Msg("Failed to create question in service")
		return nil, err
	}
	var resp dto.QuestionResponse
	copier.Copy(&resp, &question)
	return &resp, nil
}

func (s *questionService) GetQuestion(id uint) (*dto.QuestionResponse, error) {
	question, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	var resp dto.QuestionResponse
	copier.Copy(&resp, question)
	return &resp, nil
}

func (s *questionService) GetAllQuestions(testID *uint) ([]dto.QuestionResponse, error) {
	var questions []model.Question
	var err error

	if testID != nil {
		questions, err = s.repo.FindByTestID(*testID)
	} else {
		questions, err = s.repo.FindAll() // Or filter for standalone questions: `s.repo.FindStandalone()`
	}

	if err != nil {
		return nil, err
	}
	var resp []dto.QuestionResponse
	copier.Copy(&resp, &questions)
	return resp, nil
}

func (s *questionService) UpdateQuestion(id uint, req dto.CreateQuestionRequest) (*dto.QuestionResponse, error) {
	question, err := s.repo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("question not found with ID %d", id)
	}
	if req.Type == "sentence_picture" && (req.ImageURL == nil || req.GivenWord1 == nil || req.GivenWord2 == nil) {
		return nil, fmt.Errorf("ImageURL, GivenWord1, and GivenWord2 are required for sentence_picture type update if type is sentence_picture")
	}

	// Update fields from request
	copier.Copy(question, &req) // This will update all fields including associations if not careful
	// Better to selectively update:
	// question.Title = req.Title
	// question.Prompt = req.Prompt ... etc.
	// For this case, copier is fine as CreateQuestionRequest has the relevant fields.

	if req.TestID != nil { // If TestID is being changed or set
		_, err := s.testRepo.FindByID(*req.TestID)
		if err != nil {
			return nil, fmt.Errorf("invalid TestID %d for update: %w", *req.TestID, err)
		}
		question.TestID = req.TestID
	} else {
		question.TestID = nil // Allow unsetting TestID
	}

	if err := s.repo.Update(question); err != nil {
		return nil, err
	}
	var resp dto.QuestionResponse
	copier.Copy(&resp, question)
	return &resp, nil
}

func (s *questionService) DeleteQuestion(id uint) error {
	// Add checks, e.g., if question is part of an attempt, maybe prevent deletion or handle carefully
	return s.repo.Delete(id)
}

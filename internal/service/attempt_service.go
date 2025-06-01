package service

import (
	"fmt"
	"sync"
	"time"

	"github.com/jinzhu/copier"
	"github.com/lshigami/Ringtails/internal/dto"
	"github.com/lshigami/Ringtails/internal/model"
	"github.com/lshigami/Ringtails/internal/repository"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type AttemptService interface {
	SubmitAttempt(req dto.SubmitAttemptRequest) (*dto.AttemptResponse, error)
	GetAttempt(id uint) (*dto.AttemptResponse, error)
	GetAllAttempts(userID *uint, testID *uint) ([]dto.AttemptResponse, error)
	SubmitFullTestAnswers(testID uint, req dto.SubmitFullTestRequest, db *gorm.DB) (*dto.SubmitFullTestResponse, error)
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

func (s *attemptService) GetAllAttempts(userID *uint, testID *uint) ([]dto.AttemptResponse, error) {
	// Truyền cả userID và testID vào repo
	attempts, err := s.attemptRepo.FindAllWithQuestions(userID, testID)
	if err != nil {
		return nil, err
	}
	var resp []dto.AttemptResponse
	copier.Copy(&resp, &attempts)
	return resp, nil
}

type feedbackResult struct {
	attempt model.Attempt // Attempt đã có hoặc chưa có AIFeedback (nếu lỗi)
	err     error         // Lỗi khi lấy feedback hoặc cập nhật DB sau đó
}

func (s *attemptService) SubmitFullTestAnswers(testID uint, req dto.SubmitFullTestRequest, db *gorm.DB) (*dto.SubmitFullTestResponse, error) {
	testQuestions, err := s.questionRepo.FindByTestID(testID)
	if err != nil {
		log.Error().Err(err).Uint("testID", testID).Msg("Failed to find test or its questions for full submission")
		return nil, fmt.Errorf("test with ID %d not found or error fetching its questions: %w", testID, err)
	}
	if len(testQuestions) == 0 {
		return nil, fmt.Errorf("test with ID %d has no questions", testID)
	}

	validQuestionModels := make(map[uint]*model.Question) // Để truy cập model Question gốc
	for i := range testQuestions {
		q := testQuestions[i]
		validQuestionModels[q.ID] = &q
	}

	var initialAttempts []model.Attempt // Lưu các attempt được tạo trong transaction
	var submissionErrors []string

	currentUserID := req.UserID
	if currentUserID == nil {
		log.Info().Uint("testID", testID).Msg("No UserID provided for full test submission.")
	}

	// 1. Transaction để tạo các bản ghi Attempt ban đầu (chưa có AI feedback)
	txErr := db.Transaction(func(tx *gorm.DB) error {
		for _, answerSubmission := range req.Answers {
			_, isValidQuestion := validQuestionModels[answerSubmission.QuestionID]
			if !isValidQuestion {
				errMsg := fmt.Sprintf("Question ID %d is not part of Test ID %d.", answerSubmission.QuestionID, testID)
				log.Warn().Str("error", errMsg).Msg("Invalid question ID in full test submission.")
				submissionErrors = append(submissionErrors, errMsg)
				continue
			}

			attempt := model.Attempt{
				UserID:      currentUserID,
				QuestionID:  answerSubmission.QuestionID,
				UserAnswer:  answerSubmission.UserAnswer,
				SubmittedAt: time.Now(),
				// AIFeedback sẽ được điền sau
			}

			if err := tx.Create(&attempt).Error; err != nil {
				log.Error().Err(err).Msg("Failed to create attempt in DB during full test submission transaction")
				return fmt.Errorf("failed to create attempt for question %d: %w", answerSubmission.QuestionID, err)
			}
			initialAttempts = append(initialAttempts, attempt) // Thêm vào slice để xử lý feedback sau
		}
		return nil // Commit transaction
	})

	if txErr != nil {
		return nil, txErr // Lỗi từ transaction, không làm gì thêm
	}

	if len(initialAttempts) == 0 && len(req.Answers) > 0 {
		// Tất cả các câu trả lời đều không hợp lệ, không có attempt nào được tạo
		return &dto.SubmitFullTestResponse{
			TestID:         testID,
			UserID:         currentUserID,
			SubmittedCount: 0,
			Attempts:       []dto.AttemptResponse{},
			Errors:         submissionErrors,
		}, nil
	}

	// 2. Lấy AI Feedback song song và cập nhật Attempts
	var wg sync.WaitGroup
	resultsChan := make(chan feedbackResult, len(initialAttempts))
	finalProcessedAttempts := make([]model.Attempt, 0, len(initialAttempts))

	for _, attemptToProcess := range initialAttempts {
		wg.Add(1)
		go func(currentAttempt model.Attempt) {
			defer wg.Done()

			questionModel, ok := validQuestionModels[currentAttempt.QuestionID]
			if !ok { // Should not happen if initial validation was correct
				resultsChan <- feedbackResult{attempt: currentAttempt, err: fmt.Errorf("internal error: question model not found for ID %d", currentAttempt.QuestionID)}
				return
			}

			aiFeedback, feedbackErr := s.geminiSvc.GetFeedbackForAttempt(questionModel, currentAttempt.UserAnswer)
			if feedbackErr != nil {
				log.Error().Err(feedbackErr).Uint("attemptID", currentAttempt.ID).Msg("Failed to get AI feedback in goroutine")
				currentAttempt.AIFeedback = "Error retrieving AI feedback: " + feedbackErr.Error()
				// Ghi nhận lỗi này để có thể báo cáo lại
			} else {
				currentAttempt.AIFeedback = aiFeedback
			}

			// Cập nhật attempt trong DB với AI feedback (operation riêng lẻ)
			// Sử dụng db instance gốc, không phải tx từ transaction đã commit
			if errUpdate := s.attemptRepo.Update(&currentAttempt); errUpdate != nil {
				log.Error().Err(errUpdate).Uint("attemptID", currentAttempt.ID).Msg("Failed to update attempt with AI feedback in goroutine")
				// Gửi lỗi này qua channel để có thể được ghi nhận
				resultsChan <- feedbackResult{attempt: currentAttempt, err: fmt.Errorf("failed to save feedback for question %d: %w", currentAttempt.QuestionID, errUpdate)}
				return
			}

			// Reload attempt để đảm bảo có Question preloaded cho response
			reloadedAttempt, errReload := s.attemptRepo.FindByID(currentAttempt.ID)
			if errReload != nil {
				log.Warn().Err(errReload).Uint("attemptID", currentAttempt.ID).Msg("Failed to reload attempt after feedback update, using current state.")
				resultsChan <- feedbackResult{attempt: currentAttempt, err: nil} // Gửi attempt hiện tại nếu reload lỗi
			} else {
				resultsChan <- feedbackResult{attempt: *reloadedAttempt, err: nil}
			}

		}(attemptToProcess)
	}

	// Đóng channel khi tất cả goroutines hoàn thành (không cần thiết nếu chỉ đọc số lượng item mong đợi)
	// go func() {
	// 	wg.Wait()
	// 	close(resultsChan)
	// }()

	// Thu thập kết quả từ channel
	// Đợi cho đến khi tất cả các goroutines hoàn thành hoặc nhận đủ số lượng kết quả
	processedCount := 0
	for processedCount < len(initialAttempts) {
		result := <-resultsChan
		if result.err != nil {
			// Thêm lỗi vào submissionErrors
			submissionErrors = append(submissionErrors, result.err.Error())
			// Vẫn thêm attempt (có thể có AIFeedback là thông báo lỗi) vào danh sách cuối cùng
		}
		finalProcessedAttempts = append(finalProcessedAttempts, result.attempt)
		processedCount++
	}
	close(resultsChan) // Đóng channel sau khi đã đọc hết

	var createdAttemptDTOs []dto.AttemptResponse
	copier.Copy(&createdAttemptDTOs, &finalProcessedAttempts)

	return &dto.SubmitFullTestResponse{
		TestID:         testID,
		UserID:         currentUserID,
		SubmittedCount: len(finalProcessedAttempts), // Hoặc len(createdAttemptDTOs)
		Attempts:       createdAttemptDTOs,
		Errors:         submissionErrors,
	}, nil
}

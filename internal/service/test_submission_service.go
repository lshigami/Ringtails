package service

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jinzhu/copier"
	"github.com/lshigami/Ringtails/internal/dto" // Corrected DTO path
	"github.com/lshigami/Ringtails/internal/model"
	"github.com/lshigami/Ringtails/internal/repository"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type TestSubmissionService interface {
	SubmitTest(testID uint, req dto.TestAttemptSubmitDTO, db *gorm.DB) (*dto.TestAttemptDetailDTO, error)
	GetTestAttemptDetails(attemptID uint) (*dto.TestAttemptDetailDTO, error)
	GetUserAttemptsForTest(testID uint, userID *uint) ([]dto.TestAttemptSummaryDTO, error)
}

type testSubmissionService struct {
	testRepo        repository.TestRepository
	questionRepo    repository.QuestionRepository // Used to get question details for AI
	testAttemptRepo repository.TestAttemptRepository
	answerRepo      repository.AnswerRepository
	geminiService   GeminiLLMService // Using the renamed service
	db              *gorm.DB         // For managing transactions directly if needed
}

func NewTestSubmissionService(
	testRepo repository.TestRepository,
	questionRepo repository.QuestionRepository,
	testAttemptRepo repository.TestAttemptRepository,
	answerRepo repository.AnswerRepository,
	geminiService GeminiLLMService,
	db *gorm.DB,
) TestSubmissionService {
	return &testSubmissionService{
		testRepo:        testRepo,
		questionRepo:    questionRepo,
		testAttemptRepo: testAttemptRepo,
		answerRepo:      answerRepo,
		geminiService:   geminiService,
		db:              db,
	}
}

type answerProcessingResult struct {
	processedAnswer model.Answer // The answer after attempting to get AI feedback and score
	originalIndex   int          // To maintain order if needed, though GORM relations handle it
	err             error        // Error during AI processing or DB update of the answer
}

func (s *testSubmissionService) SubmitTest(testID uint, req dto.TestAttemptSubmitDTO, db *gorm.DB) (*dto.TestAttemptDetailDTO, error) {
	// 1. Validate Test and prepare question map
	test, err := s.testRepo.FindByIDWithQuestions(testID)
	if err != nil {
		log.Error().Err(err).Uint("testID", testID).Msg("SubmitTest: Test not found")
		return nil, fmt.Errorf("test not found with ID %d: %w", testID, err)
	}
	if len(test.Questions) == 0 {
		return nil, fmt.Errorf("test ID %d has no questions, cannot submit", testID)
	}
	questionMap := make(map[uint]model.Question)
	for _, q := range test.Questions {
		questionMap[q.ID] = q
	}

	// 2. Create initial TestAttempt and Answer records in a transaction
	testAttempt := model.TestAttempt{
		TestID:      testID,
		UserID:      req.UserID, // Will be nil if not provided in DTO
		SubmittedAt: time.Now(),
		Status:      "pending", // Initial status before scoring
	}

	for _, userAnswerDto := range req.Answers {
		question, exists := questionMap[userAnswerDto.QuestionID]
		if !exists {
			log.Warn().Uint("questionID", userAnswerDto.QuestionID).Msg("SubmitTest: Submitted answer for a question not in this test, skipping.")
			continue // Or return an error: fmt.Errorf("question ID %d not part of test %d", userAnswerDto.QuestionID, testID)
		}
		testAttempt.Answers = append(testAttempt.Answers, model.Answer{
			QuestionID: question.ID,
			UserAnswer: userAnswerDto.UserAnswer,
			// AIScore and AIFeedback will be populated later
		})
	}

	if len(testAttempt.Answers) == 0 {
		return nil, fmt.Errorf("no valid answers provided for the questions in test %d", testID)
	}

	// Transaction for creating TestAttempt and its initial Answers
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&testAttempt).Error; err != nil {
			return fmt.Errorf("failed to create test attempt record: %w", err)
		}
		// After Create, testAttempt.ID and testAttempt.Answers[i].ID are populated
		return nil
	})
	if err != nil {
		log.Error().Err(err).Msg("SubmitTest: Transaction failed for creating test attempt.")
		return nil, err
	}

	// Update status to "scoring" immediately after successful creation
	testAttempt.Status = "scoring"
	if errStatusUpdate := s.testAttemptRepo.Update(&testAttempt); errStatusUpdate != nil {
		log.Error().Err(errStatusUpdate).Uint("attemptID", testAttempt.ID).Msg("SubmitTest: Failed to update test attempt status to 'scoring'")
		// Non-fatal for the submission process, but should be monitored.
	}

	// 3. Process answers for AI feedback and scoring in parallel
	var wg sync.WaitGroup
	// Use the answers from the persisted testAttempt, as they now have IDs.
	persistedAnswers := make([]model.Answer, len(testAttempt.Answers))
	copy(persistedAnswers, testAttempt.Answers) // Make a fresh copy to avoid race conditions if GORM updates the slice

	resultsChan := make(chan answerProcessingResult, len(persistedAnswers))

	for i := 0; i < len(persistedAnswers); i++ {
		wg.Add(1)
		go func(answerIdx int) { // Pass index to access the correct persistedAnswer
			defer wg.Done()

			currentAnswer := persistedAnswers[answerIdx]           // Work on a copy for this goroutine
			questionModel := questionMap[currentAnswer.QuestionID] // Get the full question model

			log.Info().Uint("answerID", currentAnswer.ID).Uint("questionID", questionModel.ID).Msg("SubmitTest: Goroutine processing answer with AI")
			feedback, score, geminiErr := s.geminiService.ScoreAndFeedbackAnswer(&questionModel, currentAnswer.UserAnswer)

			currentAnswer.AIFeedback = feedback
			if geminiErr != nil {
				log.Error().Err(geminiErr).Uint("answerID", currentAnswer.ID).Msg("SubmitTest: Error from Gemini service for answer")
				currentAnswer.AIScore = nil // Explicitly nil on error
			} else {
				currentAnswer.AIScore = &score
			}

			// Update the individual Answer record in the database
			if updateErr := s.answerRepo.Update(&currentAnswer); updateErr != nil {
				log.Error().Err(updateErr).Uint("answerID", currentAnswer.ID).Msg("SubmitTest: Failed to update answer with AI results")
				resultsChan <- answerProcessingResult{processedAnswer: currentAnswer, originalIndex: answerIdx, err: updateErr}
				return
			}
			resultsChan <- answerProcessingResult{processedAnswer: currentAnswer, originalIndex: answerIdx, err: nil}
		}(i)
	}

	// Collect results and update the original persistedAnswers slice
	var processingErrors []string
	totalScore := 0.0
	allAnswersProcessedSuccessfully := true

	for i := 0; i < len(persistedAnswers); i++ {
		result := <-resultsChan
		persistedAnswers[result.originalIndex] = result.processedAnswer // Update the master slice
		if result.err != nil {
			allAnswersProcessedSuccessfully = false
			errMsg := fmt.Sprintf("Error processing answer for question ID %d: %s", result.processedAnswer.QuestionID, result.err.Error())
			processingErrors = append(processingErrors, errMsg)
			log.Warn().Err(result.err).Uint("answerID", result.processedAnswer.ID).Msg("An error occurred while processing an answer in parallel.")
		} else if result.processedAnswer.AIScore != nil {
			totalScore += *result.processedAnswer.AIScore
		}
	}
	close(resultsChan) // Close channel after all results are received

	// 4. Update TestAttempt with total score and final status
	testAttempt.TotalScore = &totalScore
	if !allAnswersProcessedSuccessfully || len(processingErrors) > 0 {
		testAttempt.Status = "completed_with_errors"
	} else {
		testAttempt.Status = "completed"
	}

	// Assign the processed answers back to the main testAttempt object
	// This is important if the TestAttempt object itself is used for the response.
	testAttempt.Answers = persistedAnswers

	if err := s.testAttemptRepo.Update(&testAttempt); err != nil {
		log.Error().Err(err).Uint("attemptID", testAttempt.ID).Msg("SubmitTest: Failed to update test attempt with total score and final status")
		// This is a significant error as the final state isn't saved.
		// Consider how to handle this - maybe return the current state with an additional error message.
		processingErrors = append(processingErrors, fmt.Sprintf("Failed to save final attempt status: %s", err.Error()))
	}

	// 5. Prepare and return response DTO by reloading the complete attempt
	detailedAttempt, reloadErr := s.testAttemptRepo.FindByIDWithDetails(testAttempt.ID)
	if reloadErr != nil {
		log.Error().Err(reloadErr).Uint("attemptID", testAttempt.ID).Msg("SubmitTest: Failed to reload detailed test attempt for response. Returning current data.")
		// Fallback to constructing DTO from `testAttempt` which now has processed answers
		var fallbackResp dto.TestAttemptDetailDTO
		copier.Copy(&fallbackResp, &testAttempt)
		fallbackResp.TestTitle = test.Title
		// Manually ensure questions in answers are populated if copier misses them
		for i, ans := range fallbackResp.Answers {
			if ans.Question.ID == 0 { // If question details are missing
				qModel := questionMap[ans.QuestionID]
				copier.Copy(&fallbackResp.Answers[i].Question, &qModel)
			}
		}
		return &fallbackResp, nil // Or return reloadErr if it's critical
	}

	// Sort answers in detailedAttempt.Answers by Question.OrderInTest for consistent FE display
	// This ensures the DTO response has answers in the question order of the test.
	sort.SliceStable(detailedAttempt.Answers, func(i, j int) bool {
		// We need the original Question models for their OrderInTest property.
		// We have `questionMap` for this.
		q_i_order := questionMap[detailedAttempt.Answers[i].QuestionID].OrderInTest
		q_j_order := questionMap[detailedAttempt.Answers[j].QuestionID].OrderInTest
		return q_i_order < q_j_order
	})

	var resp dto.TestAttemptDetailDTO
	copier.Copy(&resp, detailedAttempt)
	if detailedAttempt.Test.ID != 0 { // Ensure TestTitle from preloaded Test
		resp.TestTitle = detailedAttempt.Test.Title
	} else {
		resp.TestTitle = test.Title // Fallback to test title fetched earlier
	}

	// Double check Question details in AnswerResponseDTO
	for i, ansModel := range detailedAttempt.Answers {
		if resp.Answers[i].Question.ID == 0 && ansModel.Question.ID != 0 {
			var qDTO dto.QuestionResponseDTO
			copier.Copy(&qDTO, &ansModel.Question) // ansModel.Question should be preloaded by FindByIDWithDetails
			resp.Answers[i].Question = qDTO
		}
	}

	return &resp, nil
}

func (s *testSubmissionService) GetTestAttemptDetails(attemptID uint) (*dto.TestAttemptDetailDTO, error) {
	attempt, err := s.testAttemptRepo.FindByIDWithDetails(attemptID)
	if err != nil {
		log.Error().Err(err).Uint("attemptID", attemptID).Msg("GetTestAttemptDetails: Failed to find test attempt")
		return nil, fmt.Errorf("test attempt not found with ID %d: %w", attemptID, err)
	}

	// To sort answers by question order, we need the questions of the test.
	// The `attempt.Test` should be preloaded. If it also has `attempt.Test.Questions` preloaded, we can use that.
	// Otherwise, we might need an additional fetch for test questions if not already available.
	questionMap := make(map[uint]model.Question)
	var testQuestions []model.Question

	if attempt.Test.ID != 0 && len(attempt.Test.Questions) > 0 {
		testQuestions = attempt.Test.Questions // Questions were preloaded with the Test
	} else if attempt.Test.ID != 0 { // Test was preloaded, but not its Questions
		log.Warn().Uint("testID", attempt.Test.ID).Msg("GetTestAttemptDetails: Test preloaded but questions were not, fetching questions separately.")
		fetchedQs, fetchErr := s.questionRepo.FindByTestID(attempt.Test.ID)
		if fetchErr != nil {
			log.Error().Err(fetchErr).Uint("testID", attempt.Test.ID).Msg("GetTestAttemptDetails: Could not fetch questions for sorting answers.")
			// Proceed without sorting or return error, for now, proceed without sorting
		} else {
			testQuestions = fetchedQs
		}
	} else { // Test itself was not preloaded (should not happen with FindByIDWithDetails)
		log.Warn().Uint("attemptID", attemptID).Msg("GetTestAttemptDetails: Test was not preloaded for attempt, cannot sort answers by question order easily.")
	}

	if len(testQuestions) > 0 {
		for _, q := range testQuestions {
			questionMap[q.ID] = q
		}
		sort.SliceStable(attempt.Answers, func(i, j int) bool {
			q_i, ok_i := questionMap[attempt.Answers[i].QuestionID]
			q_j, ok_j := questionMap[attempt.Answers[j].QuestionID]
			if !ok_i || !ok_j {
				return false
			} // Should not happen if data is consistent
			return q_i.OrderInTest < q_j.OrderInTest
		})
	}

	var resp dto.TestAttemptDetailDTO
	if err := copier.Copy(&resp, attempt); err != nil {
		log.Error().Err(err).Msg("GetTestAttemptDetails: Failed to copy attempt model to DTO")
		return nil, fmt.Errorf("error preparing response: %w", err)
	}
	if attempt.Test.ID != 0 {
		resp.TestTitle = attempt.Test.Title
	}
	// Ensure nested Question details in AnswerResponseDTO are populated
	for i, ansModel := range attempt.Answers {
		if resp.Answers[i].Question.ID == 0 && ansModel.Question.ID != 0 {
			// ansModel.Question should have been preloaded by FindByIDWithDetails
			var qDTO dto.QuestionResponseDTO
			copier.Copy(&qDTO, &ansModel.Question)
			resp.Answers[i].Question = qDTO
		}
	}

	return &resp, nil
}

func (s *testSubmissionService) GetUserAttemptsForTest(testID uint, userID *uint) ([]dto.TestAttemptSummaryDTO, error) {
	attempts, err := s.testAttemptRepo.FindAllByTestAndUser(testID, userID)
	if err != nil {
		log.Error().Err(err).Uint("testID", testID).Interface("userID", userID).Msg("GetUserAttemptsForTest: Failed to find attempts")
		return nil, fmt.Errorf("error fetching attempts for test %d: %w", testID, err)
	}
	var dtos []dto.TestAttemptSummaryDTO
	if err := copier.Copy(&dtos, &attempts); err != nil {
		log.Error().Err(err).Msg("GetUserAttemptsForTest: Failed to copy attempt models to DTOs")
		return nil, fmt.Errorf("error preparing response data: %w", err)
	}
	return dtos, nil
}

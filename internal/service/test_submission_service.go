package service

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jinzhu/copier"
	"github.com/lshigami/Ringtails/internal/dto" // Ensure this path is correct
	"github.com/lshigami/Ringtails/internal/model"
	"github.com/lshigami/Ringtails/internal/repository"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// TestSubmissionService defines the interface for managing test submissions.
type TestSubmissionService interface {
	SubmitTest(testID uint, req dto.TestAttemptSubmitDTO) (*dto.TestAttemptDetailDTO, error) // Removed db from interface
	GetTestAttemptDetails(attemptID uint) (*dto.TestAttemptDetailDTO, error)
	GetUserAttemptsForTest(testID uint, userID *uint) ([]dto.TestAttemptSummaryDTO, error)
}

type testSubmissionService struct {
	testRepo        repository.TestRepository
	questionRepo    repository.QuestionRepository
	testAttemptRepo repository.TestAttemptRepository
	answerRepo      repository.AnswerRepository
	geminiService   GeminiLLMService
	scoreConverter  ScoreConverterService
	db              *gorm.DB // Used for transactions within service methods
}

// NewTestSubmissionService creates a new instance of TestSubmissionService.
func NewTestSubmissionService(
	testRepo repository.TestRepository,
	questionRepo repository.QuestionRepository,
	testAttemptRepo repository.TestAttemptRepository,
	answerRepo repository.AnswerRepository,
	geminiService GeminiLLMService,
	scoreConverter ScoreConverterService,
	db *gorm.DB,
) TestSubmissionService {
	return &testSubmissionService{
		testRepo:        testRepo,
		questionRepo:    questionRepo,
		testAttemptRepo: testAttemptRepo,
		answerRepo:      answerRepo,
		geminiService:   geminiService,
		scoreConverter:  scoreConverter,
		db:              db,
	}
}

// answerProcessingResult is used to carry results from goroutines processing answers.
type answerProcessingResult struct {
	processedAnswer model.Answer
	originalIndex   int   // To map back to the original answer order if needed
	err             error // Error during AI processing or DB update of this specific answer
}

// SubmitTest handles the submission of answers for an entire test.
func (s *testSubmissionService) SubmitTest(testID uint, req dto.TestAttemptSubmitDTO) (*dto.TestAttemptDetailDTO, error) {
	// 1. Validate Test and prepare question map
	test, err := s.testRepo.FindByIDWithQuestions(testID)
	if err != nil {
		log.Error().Err(err).Uint("testID", testID).Msg("SubmitTest: Test not found")
		return nil, fmt.Errorf("test not found with ID %d: %w", testID, err)
	}
	if len(test.Questions) == 0 {
		return nil, fmt.Errorf("test ID %d has no questions, submission is not possible", testID)
	}
	questionMap := make(map[uint]model.Question)
	for _, q := range test.Questions {
		questionMap[q.ID] = q
	}

	// 2. Create initial TestAttempt and Answer records
	testAttempt := model.TestAttempt{
		TestID:      testID,
		UserID:      req.UserID,
		SubmittedAt: time.Now(),
		Status:      "pending", // Initial status before AI scoring
	}

	validAnswersToProcess := 0
	for _, userAnswerDto := range req.Answers {
		question, exists := questionMap[userAnswerDto.QuestionID]
		if !exists {
			log.Warn().Uint("questionID", userAnswerDto.QuestionID).Uint("testID", testID).Msg("SubmitTest: Submitted answer for a question not part of this test, skipping.")
			continue
		}
		testAttempt.Answers = append(testAttempt.Answers, model.Answer{
			QuestionID: question.ID,
			UserAnswer: userAnswerDto.UserAnswer,
		})
		validAnswersToProcess++
	}

	if validAnswersToProcess == 0 {
		return nil, fmt.Errorf("no valid answers provided for the questions in test %d", testID)
	}

	// Transaction for creating TestAttempt and its initial (unscored) Answers
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&testAttempt).Error; err != nil { // GORM creates associated answers
			return fmt.Errorf("failed to create test attempt record: %w", err)
		}
		// After Create, testAttempt.ID and testAttempt.Answers[i].ID are populated.
		return nil
	})
	if err != nil {
		log.Error().Err(err).Msg("SubmitTest: Transaction failed for creating test attempt and initial answers.")
		return nil, err
	}

	// Update status to "scoring" after successful creation of records
	testAttempt.Status = "scoring"
	if errStatusUpdate := s.testAttemptRepo.Update(&testAttempt); errStatusUpdate != nil {
		log.Error().Err(errStatusUpdate).Uint("attemptID", testAttempt.ID).Msg("SubmitTest: Failed to update test attempt status to 'scoring'. Scoring will proceed.")
		// This is not fatal for the scoring process itself but indicates a state update issue.
	}

	// 3. Process answers for AI feedback and scoring in parallel
	var wg sync.WaitGroup
	// Use a fresh copy of answers from the created testAttempt which now have IDs
	persistedAnswers := make([]model.Answer, len(testAttempt.Answers))
	copy(persistedAnswers, testAttempt.Answers)

	resultsChan := make(chan answerProcessingResult, len(persistedAnswers))

	for i := 0; i < len(persistedAnswers); i++ {
		wg.Add(1)
		go func(answerIdx int) {
			defer wg.Done()

			currentAnswer := persistedAnswers[answerIdx] // Each goroutine works on its copy
			questionModel := questionMap[currentAnswer.QuestionID]

			log.Info().Uint("answerID", currentAnswer.ID).Uint("questionID", questionModel.ID).Msg("SubmitTest: Goroutine processing answer with AI.")
			feedback, score, geminiErr := s.geminiService.ScoreAndFeedbackAnswer(&questionModel, currentAnswer.UserAnswer)

			currentAnswer.AIFeedback = feedback
			if geminiErr != nil {
				log.Error().Err(geminiErr).Uint("answerID", currentAnswer.ID).Msg("SubmitTest: Error from Gemini service for answer.")
				currentAnswer.AIScore = nil // Explicitly set AIScore to nil on error
			} else {
				currentAnswer.AIScore = &score
			}

			// Update the individual Answer record in the database
			if updateErr := s.answerRepo.Update(&currentAnswer); updateErr != nil {
				log.Error().Err(updateErr).Uint("answerID", currentAnswer.ID).Msg("SubmitTest: Failed to update answer with AI results.")
				resultsChan <- answerProcessingResult{processedAnswer: currentAnswer, originalIndex: answerIdx, err: updateErr}
				return
			}
			resultsChan <- answerProcessingResult{processedAnswer: currentAnswer, originalIndex: answerIdx, err: nil}
		}(i)
	}

	// Collect results from goroutines
	var processingErrors []string
	totalRawScore := 0.0
	allAnswersScoredSuccessfully := true // Assume success initially

	// Use a temporary slice to store processed answers in their original order
	finalOrderedAnswers := make([]model.Answer, len(persistedAnswers))

	for i := 0; i < len(persistedAnswers); i++ {
		result := <-resultsChan
		finalOrderedAnswers[result.originalIndex] = result.processedAnswer // Place in correct order
		if result.err != nil {
			allAnswersScoredSuccessfully = false
			errMsg := fmt.Sprintf("Error processing answer for question ID %d (Answer ID: %d): %s", result.processedAnswer.QuestionID, result.processedAnswer.ID, result.err.Error())
			processingErrors = append(processingErrors, errMsg)
			log.Warn().Err(result.err).Uint("answerID", result.processedAnswer.ID).Msg("An error occurred while an answer was being processed by AI/DB.")
		} else if result.processedAnswer.AIScore != nil {
			totalRawScore += *result.processedAnswer.AIScore
		}
	}
	close(resultsChan) // Close channel after all results are received.

	// 4. Update TestAttempt with total raw score and final status
	testAttempt.TotalScore = &totalRawScore // This is the Total Raw Score
	if !allAnswersScoredSuccessfully || len(processingErrors) > 0 {
		testAttempt.Status = "completed_with_errors"
	} else {
		testAttempt.Status = "completed"
	}
	// Assign the accurately ordered and processed answers back
	testAttempt.Answers = finalOrderedAnswers

	if err := s.testAttemptRepo.Update(&testAttempt); err != nil {
		log.Error().Err(err).Uint("attemptID", testAttempt.ID).Msg("SubmitTest: Failed to update test attempt with total score and final status.")
		processingErrors = append(processingErrors, fmt.Sprintf("Critical: Failed to save final attempt status and total score: %s", err.Error()))
		// Even if this fails, try to return the best possible DTO
	}

	// 5. Prepare and return response DTO
	// We reload the TestAttempt to get all associations correctly populated by GORM for the DTO.
	// This also ensures we return the most up-to-date state from the DB.
	detailedAttempt, reloadErr := s.testAttemptRepo.FindByIDWithDetails(testAttempt.ID)
	if reloadErr != nil {
		log.Error().Err(reloadErr).Uint("attemptID", testAttempt.ID).Msg("SubmitTest: Failed to reload detailed test attempt for response. Constructing DTO from current state.")
		// Fallback: Construct DTO from the `testAttempt` variable if reload fails
		// This `testAttempt` already has the updated answers, total score, and status.
		var fallbackResp dto.TestAttemptDetailDTO
		if errCopy := copier.Copy(&fallbackResp, &testAttempt); errCopy != nil {
			log.Error().Err(errCopy).Msg("SubmitTest: Error copying fallback TestAttempt to DTO")
			return nil, fmt.Errorf("error preparing fallback response: %w", errCopy)
		}
		fallbackResp.TestTitle = test.Title // Add test title from initially fetched test
		// Manually populate Question details within answers if copier misses them
		answerDTOs := make([]dto.AnswerResponseDTO, len(testAttempt.Answers))
		for i, ansModel := range testAttempt.Answers {
			copier.Copy(&answerDTOs[i], &ansModel)
			qModel := questionMap[ansModel.QuestionID]
			copier.Copy(&answerDTOs[i].Question, &qModel)
		}
		fallbackResp.Answers = answerDTOs

		if testAttempt.TotalScore != nil {
			scaledScore, errScale := s.scoreConverter.ConvertToScaledScore(*testAttempt.TotalScore)
			if errScale == nil {
				fallbackResp.ScaledScore = &scaledScore
			}
		}
		fallbackResp.TotalRawScore = testAttempt.TotalScore
		return &fallbackResp, nil
	}

	// Sort answers in the reloaded detailedAttempt for consistent DTO response
	if len(detailedAttempt.Answers) > 0 && len(questionMap) > 0 {
		sort.SliceStable(detailedAttempt.Answers, func(i, j int) bool {
			q_i, ok_i := questionMap[detailedAttempt.Answers[i].QuestionID]
			q_j, ok_j := questionMap[detailedAttempt.Answers[j].QuestionID]
			if !ok_i || !ok_j {
				return false
			}
			return q_i.OrderInTest < q_j.OrderInTest
		})
	}

	var resp dto.TestAttemptDetailDTO
	if err := copier.Copy(&resp, detailedAttempt); err != nil {
		log.Error().Err(err).Msg("SubmitTest: Error copying reloaded TestAttempt to DTO.")
		return nil, fmt.Errorf("error preparing final response: %w", err)
	}
	// Ensure TestTitle is correctly set from the preloaded Test within detailedAttempt
	if detailedAttempt.Test.ID != 0 {
		resp.TestTitle = detailedAttempt.Test.Title
	} else {
		resp.TestTitle = test.Title // Fallback, though detailedAttempt.Test should be loaded
	}

	resp.TotalRawScore = detailedAttempt.TotalScore // Set raw score
	if detailedAttempt.TotalScore != nil {
		scaledScore, errScale := s.scoreConverter.ConvertToScaledScore(*detailedAttempt.TotalScore)
		if errScale != nil {
			log.Warn().Err(errScale).Float64("rawScore", *detailedAttempt.TotalScore).Msg("SubmitTest: Failed to scale score for final response DTO.")
		} else {
			resp.ScaledScore = &scaledScore
		}
	}

	// Ensure Question details within AnswerResponseDTO are fully populated
	resp.Answers = make([]dto.AnswerResponseDTO, len(detailedAttempt.Answers))
	for i, ansModel := range detailedAttempt.Answers {
		var ansDTO dto.AnswerResponseDTO
		copier.Copy(&ansDTO, &ansModel)
		if ansModel.Question.ID != 0 { // ansModel.Question is preloaded by FindByIDWithDetails
			var qDTO dto.QuestionResponseDTO
			copier.Copy(&qDTO, &ansModel.Question)
			ansDTO.Question = qDTO
		} else { // Fallback if Question somehow wasn't preloaded in Answer
			qModelFromMap := questionMap[ansModel.QuestionID]
			var qDTO dto.QuestionResponseDTO
			copier.Copy(&qDTO, &qModelFromMap)
			ansDTO.Question = qDTO
		}
		resp.Answers[i] = ansDTO
	}

	return &resp, nil
}

// GetTestAttemptDetails retrieves full details for a specific test attempt.
func (s *testSubmissionService) GetTestAttemptDetails(attemptID uint) (*dto.TestAttemptDetailDTO, error) {
	attempt, err := s.testAttemptRepo.FindByIDWithDetails(attemptID)
	if err != nil {
		log.Error().Err(err).Uint("attemptID", attemptID).Msg("GetTestAttemptDetails: Failed to find test attempt by ID.")
		return nil, fmt.Errorf("test attempt not found with ID %d: %w", attemptID, err)
	}

	// Prepare questionMap for sorting answers if test questions are available
	questionMap := make(map[uint]model.Question)
	var testQuestionsToSortBy []model.Question

	if attempt.Test.ID != 0 { // Test object should be preloaded by FindByIDWithDetails
		if len(attempt.Test.Questions) > 0 {
			testQuestionsToSortBy = attempt.Test.Questions // Questions were preloaded with the Test
		} else {
			// If Test was preloaded but its Questions weren't (less likely with current FindByIDWithDetails)
			log.Warn().Uint("testID", attempt.Test.ID).Msg("GetTestAttemptDetails: Test preloaded but its Questions list is empty. Fetching questions separately for sorting.")
			fetchedQs, fetchErr := s.questionRepo.FindByTestID(attempt.Test.ID)
			if fetchErr != nil {
				log.Error().Err(fetchErr).Uint("testID", attempt.Test.ID).Msg("GetTestAttemptDetails: Could not fetch questions for sorting answers.")
			} else {
				testQuestionsToSortBy = fetchedQs
			}
		}
	}

	if len(testQuestionsToSortBy) > 0 {
		for _, q := range testQuestionsToSortBy {
			questionMap[q.ID] = q
		}
		sort.SliceStable(attempt.Answers, func(i, j int) bool {
			q_i, ok_i := questionMap[attempt.Answers[i].QuestionID]
			q_j, ok_j := questionMap[attempt.Answers[j].QuestionID]
			if !ok_i || !ok_j {
				return false
			} // Should ideally not happen
			return q_i.OrderInTest < q_j.OrderInTest
		})
	}

	var resp dto.TestAttemptDetailDTO
	if err := copier.Copy(&resp, attempt); err != nil {
		log.Error().Err(err).Msg("GetTestAttemptDetails: Failed to copy attempt model to DTO.")
		return nil, fmt.Errorf("error preparing response data: %w", err)
	}

	if attempt.Test.ID != 0 { // Set TestTitle from the preloaded Test
		resp.TestTitle = attempt.Test.Title
	}

	resp.TotalRawScore = attempt.TotalScore
	if attempt.TotalScore != nil {
		scaledScore, errScale := s.scoreConverter.ConvertToScaledScore(*attempt.TotalScore)
		if errScale != nil {
			log.Warn().Err(errScale).Float64("rawScore", *attempt.TotalScore).Msg("GetTestAttemptDetails: Failed to scale score for DTO.")
		} else {
			resp.ScaledScore = &scaledScore
		}
	}

	// Ensure Question details within AnswerResponseDTO are fully populated
	resp.Answers = make([]dto.AnswerResponseDTO, len(attempt.Answers))
	for i, ansModel := range attempt.Answers {
		var ansDTO dto.AnswerResponseDTO
		copier.Copy(&ansDTO, &ansModel)
		// ansModel.Question should be preloaded by FindByIDWithDetails
		if ansModel.Question.ID != 0 {
			var qDTO dto.QuestionResponseDTO
			copier.Copy(&qDTO, &ansModel.Question)
			ansDTO.Question = qDTO
		} else { // Fallback if Question wasn't preloaded with Answer
			qModelFromMap := questionMap[ansModel.QuestionID] // Use map built from testQuestionsToSortBy
			if qModelFromMap.ID != 0 {                        // Check if question was found in map
				var qDTO dto.QuestionResponseDTO
				copier.Copy(&qDTO, &qModelFromMap)
				ansDTO.Question = qDTO
			} else {
				log.Warn().Uint("questionID", ansModel.QuestionID).Msg("GetTestAttemptDetails: Question model not found in map for answer's question DTO.")
			}
		}
		resp.Answers[i] = ansDTO
	}

	return &resp, nil
}

// GetUserAttemptsForTest retrieves a summary list of a user's attempts for a specific test.
func (s *testSubmissionService) GetUserAttemptsForTest(testID uint, userID *uint) ([]dto.TestAttemptSummaryDTO, error) {
	attempts, err := s.testAttemptRepo.FindAllByTestAndUser(testID, userID)
	if err != nil {
		log.Error().Err(err).Uint("testID", testID).Interface("userID", userID).Msg("GetUserAttemptsForTest: Failed to find attempts from repository.")
		return nil, fmt.Errorf("error fetching attempts for test %d: %w", testID, err)
	}

	var dtos []dto.TestAttemptSummaryDTO
	for _, attempt := range attempts {
		var summary dto.TestAttemptSummaryDTO
		// Copy basic fields
		if errCp := copier.Copy(&summary, &attempt); errCp != nil {
			log.Error().Err(errCp).Uint("attemptID", attempt.ID).Msg("GetUserAttemptsForTest: Error copying attempt to summary DTO")
			continue // Skip this attempt if copying fails
		}

		summary.TotalRawScore = attempt.TotalScore // Assign raw score
		if attempt.TotalScore != nil {
			scaledScore, errScale := s.scoreConverter.ConvertToScaledScore(*attempt.TotalScore)
			if errScale != nil {
				log.Warn().Err(errScale).Float64("rawScore", *attempt.TotalScore).Msg("GetUserAttemptsForTest: Failed to scale score for summary.")
			} else {
				summary.ScaledScore = &scaledScore
			}
		}
		dtos = append(dtos, summary)
	}
	return dtos, nil
}

package service

import (
	"fmt"
	"time"

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
	GetTestAttemptHistory(testID uint, userID *uint) (*dto.TestAttemptHistoryResponseDTO, error)
	GetCompletedTestsByUser(userID *uint) ([]dto.TestResponse, error)
	GetTestAttemptsByUser(testID uint, userID *uint) (*dto.TestAttemptsResponse, error)
	GetTestAttemptsList(testID uint) (*dto.TestAttemptsListResponse, error)
	GetTestAttemptDetail(testID uint, attemptID uint) (*dto.TestAttemptDetailResponse, error)
}

type testService struct {
	testRepo     repository.TestRepository
	questionRepo repository.QuestionRepository
	attemptRepo  repository.AttemptRepository

	db *gorm.DB // For transactions
}

func NewTestService(testRepo repository.TestRepository, questionRepo repository.QuestionRepository, attemptRepo repository.AttemptRepository, db *gorm.DB) TestService {
	return &testService{testRepo: testRepo, questionRepo: questionRepo, attemptRepo: attemptRepo, db: db}
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

func (s *testService) GetTestAttemptHistory(testID uint, userID *uint) (*dto.TestAttemptHistoryResponseDTO, error) {
	// 1. Lấy thông tin Test và các Questions của nó
	test, err := s.testRepo.FindByIDWithQuestions(testID)
	if err != nil {
		log.Error().Err(err).Uint("testID", testID).Msg("Failed to find test for history")
		return nil, fmt.Errorf("test with ID %d not found: %w", testID, err)
	}
	if len(test.Questions) == 0 {
		// Trả về test không có câu hỏi nếu cần, hoặc lỗi
		return &dto.TestAttemptHistoryResponseDTO{
			TestID:           test.ID,
			TestTitle:        test.Title,
			UserID:           userID,
			QuestionsHistory: []dto.QuestionAttemptHistoryDTO{},
		}, nil
	}

	// 2. Lấy tất cả các attempts của user này cho các câu hỏi trong test này
	// Tạo danh sách các question IDs từ test.Questions
	var questionIDs []uint
	for _, q := range test.Questions {
		questionIDs = append(questionIDs, q.ID)
	}

	// Lấy tất cả attempts của user cho các question_id này
	// Chúng ta có thể sửa đổi FindAllWithQuestions để nhận một slice các questionIDs
	// Hoặc, chúng ta lấy tất cả attempts của user cho testID (dùng cách hiện tại)
	// `s.attemptRepo.FindAllWithQuestions` đã hỗ trợ lọc theo testID và userID
	userAttemptsForTest, err := s.attemptRepo.FindAllWithQuestions(userID, &testID)
	if err != nil {
		log.Error().Err(err).Uint("testID", testID).Interface("userID", userID).Msg("Failed to fetch attempts for test history")
		return nil, fmt.Errorf("could not fetch attempts for test history: %w", err)
	}

	// Nhóm các attempts theo QuestionID để dễ dàng map
	attemptsByQuestionID := make(map[uint][]model.Attempt)
	for _, attempt := range userAttemptsForTest {
		attemptsByQuestionID[attempt.QuestionID] = append(attemptsByQuestionID[attempt.QuestionID], attempt)
	}

	// 3. Xây dựng response
	var questionsHistory []dto.QuestionAttemptHistoryDTO
	for _, question := range test.Questions {
		qHistory := dto.QuestionAttemptHistoryDTO{
			QuestionID:    question.ID,
			QuestionTitle: question.Title,
			QuestionType:  question.Type,
			OrderInTest:   question.OrderInTest,
			Prompt:        question.Prompt,
			ImageURL:      question.ImageURL,
			GivenWord1:    question.GivenWord1,
			GivenWord2:    question.GivenWord2,
			Attempts:      []dto.AttemptInfoDTO{}, // Khởi tạo rỗng
		}

		if userAttempts, found := attemptsByQuestionID[question.ID]; found {
			for _, attempt := range userAttempts {
				qHistory.Attempts = append(qHistory.Attempts, dto.AttemptInfoDTO{
					AttemptID:   attempt.ID,
					UserAnswer:  attempt.UserAnswer,
					AIFeedback:  attempt.AIFeedback,
					SubmittedAt: attempt.SubmittedAt,
				})
			}
		}
		questionsHistory = append(questionsHistory, qHistory)
	}

	return &dto.TestAttemptHistoryResponseDTO{
		TestID:           test.ID,
		TestTitle:        test.Title,
		UserID:           userID,
		QuestionsHistory: questionsHistory,
	}, nil
}

// GetCompletedTestsByUser retrieves all tests that a user has attempted
func (s *testService) GetCompletedTestsByUser(userID *uint) ([]dto.TestResponse, error) {
	if userID == nil {
		return nil, fmt.Errorf("user ID is required")
	}

	// Get all attempts by this user
	attempts, err := s.attemptRepo.FindAllWithQuestions(userID, nil)
	if err != nil {
		log.Error().Err(err).Interface("userID", userID).Msg("Failed to fetch attempts for user")
		return nil, fmt.Errorf("could not fetch attempts for user: %w", err)
	}

	// Extract unique test IDs from the attempts
	testIDMap := make(map[uint]bool)
	for _, attempt := range attempts {
		if attempt.Question.TestID != nil {
			testIDMap[*attempt.Question.TestID] = true
		}
	}

	if len(testIDMap) == 0 {
		// No tests found for this user
		return []dto.TestResponse{}, nil
	}

	// Get all tests that the user has attempted
	var testResponses []dto.TestResponse
	for testID := range testIDMap {
		test, err := s.testRepo.FindByID(testID)
		if err != nil {
			log.Warn().Err(err).Uint("testID", testID).Msg("Failed to find test for user history")
			continue // Skip this test if not found
		}

		var testResp dto.TestResponse
		copier.Copy(&testResp, test)
		testResponses = append(testResponses, testResp)
	}

	return testResponses, nil
}

// GetTestAttemptsByUser retrieves all attempts for a specific test by a user
func (s *testService) GetTestAttemptsByUser(testID uint, userID *uint) (*dto.TestAttemptsResponse, error) {
	if userID == nil {
		return nil, fmt.Errorf("user ID is required")
	}

	// Get the test information
	test, err := s.testRepo.FindByID(testID)
	if err != nil {
		log.Error().Err(err).Uint("testID", testID).Msg("Failed to find test")
		return nil, fmt.Errorf("test with ID %d not found: %w", testID, err)
	}

	// Get all attempts for this test by this user
	attempts, err := s.attemptRepo.FindAllWithQuestions(userID, &testID)
	if err != nil {
		log.Error().Err(err).Uint("testID", testID).Interface("userID", userID).Msg("Failed to fetch attempts for test")
		return nil, fmt.Errorf("could not fetch attempts for test: %w", err)
	}

	if len(attempts) == 0 {
		// No attempts found for this test by this user
		return &dto.TestAttemptsResponse{
			TestID:    test.ID,
			TestTitle: test.Title,
			UserID:    userID,
			Attempts:  []dto.AttemptResponse{},
		}, nil
	}

	// Convert attempts to DTOs
	var attemptResponses []dto.AttemptResponse
	var latestSubmitTime *time.Time

	for _, attempt := range attempts {
		var attemptResp dto.AttemptResponse
		copier.Copy(&attemptResp, &attempt)

		// Keep track of the latest submission time
		if latestSubmitTime == nil || attempt.SubmittedAt.After(*latestSubmitTime) {
			latestSubmitTime = &attempt.SubmittedAt
		}

		attemptResponses = append(attemptResponses, attemptResp)
	}

	return &dto.TestAttemptsResponse{
		TestID:      test.ID,
		TestTitle:   test.Title,
		UserID:      userID,
		Attempts:    attemptResponses,
		SubmittedAt: latestSubmitTime,
	}, nil
}

// GetTestAttemptsList retrieves a list of all attempts for a specific test with basic information
func (s *testService) GetTestAttemptsList(testID uint) (*dto.TestAttemptsListResponse, error) {
	// Get the test information
	test, err := s.testRepo.FindByIDWithQuestions(testID)
	if err != nil {
		log.Error().Err(err).Uint("testID", testID).Msg("Failed to find test for attempt list")
		return nil, fmt.Errorf("test with ID %d not found: %w", testID, err)
	}

	// Get all attempts for this test
	// We need to get all attempts for all questions in this test
	var questionIDs []uint
	for _, q := range test.Questions {
		questionIDs = append(questionIDs, q.ID)
	}

	if len(questionIDs) == 0 {
		// Test has no questions
		return &dto.TestAttemptsListResponse{
			TestID:     test.ID,
			TestTitle:  test.Title,
			Attempts:   []dto.TestAttemptsListItem{},
			TotalCount: 0,
		}, nil
	}

	// Get all attempts for these questions
	// For now, we'll get all attempts for the test without filtering by user
	attempts, err := s.attemptRepo.FindAllWithQuestions(nil, &testID)
	if err != nil {
		log.Error().Err(err).Uint("testID", testID).Msg("Failed to fetch attempts for test list")
		return nil, fmt.Errorf("could not fetch attempts for test list: %w", err)
	}

	// Group attempts by user and submission time to create a list of test attempts
	// We'll use a map to track unique attempts by user and approximate submission time
	attemptsByUserAndTime := make(map[string][]model.Attempt)
	for _, attempt := range attempts {
		if attempt.UserID == nil {
			continue // Skip attempts without a user ID
		}
		
		// Create a key using user ID and rounded submission time (to group attempts made close together)
		// This is a simple approach - a more robust solution would track test sessions
		roundedTime := attempt.SubmittedAt.Truncate(5 * time.Minute) // Group attempts within 5 minutes
		key := fmt.Sprintf("%d-%s", *attempt.UserID, roundedTime.Format(time.RFC3339))
		attemptsByUserAndTime[key] = append(attemptsByUserAndTime[key], attempt)
	}

	// Create the response items
	var attemptItems []dto.TestAttemptsListItem
	for _, userAttempts := range attemptsByUserAndTime {
		if len(userAttempts) == 0 {
			continue
		}
		
		// Use the first attempt to get user info and the latest for submission time
		firstAttempt := userAttempts[0]
		latestTime := firstAttempt.SubmittedAt
		latestAttemptID := firstAttempt.ID
		
		for _, attempt := range userAttempts {
			if attempt.SubmittedAt.After(latestTime) {
				latestTime = attempt.SubmittedAt
				latestAttemptID = attempt.ID
			}
		}
		
		// Calculate score and completion metrics
		// In a real implementation, you might have a more sophisticated scoring system
		completedQuestions := len(userAttempts)
		totalQuestions := len(test.Questions)
		
		// For now, we'll use a simple score calculation
		// In a real app, you might have a more complex scoring system
		score := (completedQuestions * 100) / totalQuestions
		
		attemptItems = append(attemptItems, dto.TestAttemptsListItem{
			AttemptID:          latestAttemptID,
			UserID:             firstAttempt.UserID,
			Username:           "User", // In a real app, you would fetch the username
			SubmittedAt:        latestTime,
			Score:              &score,
			TotalQuestions:     totalQuestions,
			CompletedQuestions: completedQuestions,
		})
	}

	return &dto.TestAttemptsListResponse{
		TestID:     test.ID,
		TestTitle:  test.Title,
		Attempts:   attemptItems,
		TotalCount: len(attemptItems),
	}, nil
}

// GetTestAttemptDetail retrieves detailed information about a specific test attempt
func (s *testService) GetTestAttemptDetail(testID uint, attemptID uint) (*dto.TestAttemptDetailResponse, error) {
	// Get the test information
	test, err := s.testRepo.FindByIDWithQuestions(testID)
	if err != nil {
		log.Error().Err(err).Uint("testID", testID).Msg("Failed to find test for attempt detail")
		return nil, fmt.Errorf("test with ID %d not found: %w", testID, err)
	}

	// Get the specific attempt
	attempt, err := s.attemptRepo.FindByID(attemptID)
	if err != nil {
		log.Error().Err(err).Uint("attemptID", attemptID).Msg("Failed to find attempt")
		return nil, fmt.Errorf("attempt with ID %d not found: %w", attemptID, err)
	}

	// Verify that this attempt belongs to a question in this test
	questionBelongsToTest := false
	for _, q := range test.Questions {
		if q.ID == attempt.QuestionID {
			questionBelongsToTest = true
			break
		}
	}

	if !questionBelongsToTest {
		return nil, fmt.Errorf("attempt %d does not belong to test %d", attemptID, testID)
	}

	// Get all attempts for this test by the same user
	var userID *uint
	if attempt.UserID != nil {
		userID = attempt.UserID
	}

	userAttemptsForTest, err := s.attemptRepo.FindAllWithQuestions(userID, &testID)
	if err != nil {
		log.Error().Err(err).Uint("testID", testID).Interface("userID", userID).Msg("Failed to fetch attempts for test detail")
		return nil, fmt.Errorf("could not fetch attempts for test detail: %w", err)
	}

	// Group attempts by question
	attemptsByQuestionID := make(map[uint][]model.Attempt)
	for _, a := range userAttemptsForTest {
		attemptsByQuestionID[a.QuestionID] = append(attemptsByQuestionID[a.QuestionID], a)
	}

	// Build the questions history
	var questionsHistory []dto.QuestionAttemptHistoryDTO
	for _, question := range test.Questions {
		qHistory := dto.QuestionAttemptHistoryDTO{
			QuestionID:    question.ID,
			QuestionTitle: question.Title,
			QuestionType:  question.Type,
			OrderInTest:   question.OrderInTest,
			Prompt:        question.Prompt,
			ImageURL:      question.ImageURL,
			GivenWord1:    question.GivenWord1,
			GivenWord2:    question.GivenWord2,
			Attempts:      []dto.AttemptInfoDTO{},
		}

		if userAttempts, found := attemptsByQuestionID[question.ID]; found {
			for _, a := range userAttempts {
				qHistory.Attempts = append(qHistory.Attempts, dto.AttemptInfoDTO{
					AttemptID:   a.ID,
					UserAnswer:  a.UserAnswer,
					AIFeedback:  a.AIFeedback,
					SubmittedAt: a.SubmittedAt,
				})
			}
		}

		questionsHistory = append(questionsHistory, qHistory)
	}

	// Calculate score (simple implementation)
	totalQuestions := len(test.Questions)
	completedQuestions := 0
	for _, qHistory := range questionsHistory {
		if len(qHistory.Attempts) > 0 {
			completedQuestions++
		}
	}

	score := (completedQuestions * 100) / totalQuestions

	// Get username (in a real app, you would fetch this from a user service)
	username := "User"
	if attempt.UserID != nil {
		username = fmt.Sprintf("User %d", *attempt.UserID)
	}

	return &dto.TestAttemptDetailResponse{
		AttemptID:        attemptID,
		TestID:           test.ID,
		TestTitle:        test.Title,
		UserID:           attempt.UserID,
		Username:         username,
		SubmittedAt:      attempt.SubmittedAt,
		Score:            &score,
		TotalQuestions:   totalQuestions,
		QuestionsHistory: questionsHistory,
	}, nil
}

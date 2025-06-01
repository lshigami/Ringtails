package controller

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	// "github.com/jinzhu/copier" // Not directly used here, but services use it
	"github.com/lshigami/Ringtails/internal/dto"
	"github.com/lshigami/Ringtails/internal/service"
	"github.com/rs/zerolog/log"
)

type Controller struct {
	questionSvc service.QuestionService
	attemptSvc  service.AttemptService
	testSvc     service.TestService // Add TestService
	db          *gorm.DB
}

func NewController(qSvc service.QuestionService, attSvc service.AttemptService, tSvc service.TestService, db *gorm.DB) *Controller {
	return &Controller{
		questionSvc: qSvc,
		attemptSvc:  attSvc,
		testSvc:     tSvc,
		db:          db,
	}
}

func (ctrl *Controller) RegisterRoutes(router *gin.Engine) {
	apiV1 := router.Group("/api/v1")
	{
		// Test routes
		tests := apiV1.Group("/tests")
		tests.POST("", ctrl.CreateTestHandler)
		tests.GET("", ctrl.GetAllTestsHandler)
		tests.GET("/:id", ctrl.GetTestHandler)
		tests.PUT("/:id", ctrl.UpdateTestHandler) // Update test metadata
		tests.DELETE("/:id", ctrl.DeleteTestHandler)
		tests.POST("/:id/questions", ctrl.AddQuestionToTestHandler) // Add a question to an existing test
		tests.GET("/:id/history", ctrl.GetTestAttemptHistoryHandler)

		// Question routes (can be standalone or part of tests)
		questions := apiV1.Group("/questions")
		questions.POST("", ctrl.CreateQuestionHandler)       // Create standalone or assign to test
		questions.GET("", ctrl.GetAllQuestionsHandler)       // Get all questions, or filter by test_id query param
		questions.GET("/:id", ctrl.GetQuestionHandler)       // Get specific question
		questions.PUT("/:id", ctrl.UpdateQuestionHandler)    // Update question
		questions.DELETE("/:id", ctrl.DeleteQuestionHandler) // Delete question

		// Attempt routes
		attempts := apiV1.Group("/attempts")
		attempts.POST("", ctrl.SubmitAttemptHandler)
		attempts.GET("", ctrl.GetAllAttemptsHandler)
		attempts.GET("/:id", ctrl.GetAttemptHandler)
		tests.POST("/:id/submit", ctrl.SubmitFullTestHandler)
		
		// New routes for test results
		done := apiV1.Group("/done")
		done.GET("/:user_id", ctrl.GetCompletedTestsByUserHandler) // Get all completed tests for a user
		
		result := apiV1.Group("/result")
		result.GET("/:test_id/:user_id", ctrl.GetTestAttemptsByUserHandler) // Get all attempts for a specific test by user
	}
}

// --- Test Handlers ---

// CreateTestHandler godoc
// @Summary Create a new test
// @Description Add a new TOEIC writing test, optionally with its questions
// @Tags tests
// @Accept json
// @Produce json
// @Param test body dto.CreateTestRequest true "Test data including optional questions"
// @Success 201 {object} dto.TestResponse
// @Failure 400 {object} dto.ErrorResponse "Invalid request body"
// @Failure 500 {object} dto.ErrorResponse "Internal server error"
// @Router /tests [post]
func (ctrl *Controller) CreateTestHandler(c *gin.Context) {
	var req dto.CreateTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warn().Err(err).Msg("Failed to bind CreateTestRequest")
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	// Validation for questions within the test
	if len(req.Questions) > 0 {
		// Example: Ensure 8 questions if provided, or specific counts for each type
		// For now, service layer handles specific field requirements (e.g., ImageURL for sentence_picture)
		orders := make(map[int]bool)
		for _, q := range req.Questions {
			if q.OrderInTest < 1 || q.OrderInTest > 8 {
				c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: fmt.Sprintf("Invalid OrderInTest %d for a question. Must be between 1 and 8.", q.OrderInTest)})
				return
			}
			if orders[q.OrderInTest] {
				c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: fmt.Sprintf("Duplicate OrderInTest %d for questions.", q.OrderInTest)})
				return
			}
			orders[q.OrderInTest] = true

			if q.Type == "sentence_picture" {
				if q.ImageURL == nil || q.GivenWord1 == nil || q.GivenWord2 == nil {
					c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: fmt.Sprintf("Question with OrderInTest %d (type sentence_picture) requires ImageURL, GivenWord1, and GivenWord2.", q.OrderInTest)})
					return
				}
			}
		}
	}

	testResp, err := ctrl.testSvc.CreateTest(req)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create test")
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to create test: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, testResp)
}

// GetTestHandler godoc
// @Summary Get a test by ID with its questions
// @Description Retrieve a specific TOEIC writing test and all its associated questions
// @Tags tests
// @Produce json
// @Param id path int true "Test ID"
// @Success 200 {object} dto.TestResponse
// @Failure 400 {object} dto.ErrorResponse "Invalid ID format"
// @Failure 404 {object} dto.ErrorResponse "Test not found"
// @Failure 500 {object} dto.ErrorResponse "Internal server error"
// @Router /tests/{id} [get]
func (ctrl *Controller) GetTestHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid test ID format"})
		return
	}

	testResp, err := ctrl.testSvc.GetTestWithQuestions(uint(id))
	if err != nil {
		log.Error().Err(err).Uint64("id", id).Msg("Failed to get test")
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Test not found or error retrieving it"})
		return
	}
	c.JSON(http.StatusOK, testResp)
}

// GetAllTestsHandler godoc
// @Summary Get all tests
// @Description Retrieve all TOEIC writing tests. Use 'with_questions=true' query param to include questions.
// @Tags tests
// @Produce json
// @Param with_questions query bool false "Include questions in the response"
// @Success 200 {array} dto.TestResponse
// @Failure 500 {object} dto.ErrorResponse "Internal server error"
// @Router /tests [get]
func (ctrl *Controller) GetAllTestsHandler(c *gin.Context) {
	withQuestionsStr := c.DefaultQuery("with_questions", "false")
	withQuestions, _ := strconv.ParseBool(withQuestionsStr)

	tests, err := ctrl.testSvc.GetAllTests(withQuestions)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get all tests")
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to retrieve tests"})
		return
	}
	c.JSON(http.StatusOK, tests)
}

// UpdateTestHandler godoc
// @Summary Update a test's metadata
// @Description Update the title or description of an existing test
// @Tags tests
// @Accept json
// @Produce json
// @Param id path int true "Test ID"
// @Param test_metadata body dto.CreateTestRequest true "Test metadata to update (only Title and Description are used)"
// @Success 200 {object} dto.TestResponse
// @Failure 400 {object} dto.ErrorResponse "Invalid request body or ID"
// @Failure 404 {object} dto.ErrorResponse "Test not found"
// @Failure 500 {object} dto.ErrorResponse "Internal server error"
// @Router /tests/{id} [put]
func (ctrl *Controller) UpdateTestHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid test ID format"})
		return
	}

	var req dto.CreateTestRequest // Reusing for Title and Description
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	testResp, err := ctrl.testSvc.UpdateTest(uint(id), req)
	if err != nil {
		// Differentiate between Not Found and other errors if service returns specific errors
		log.Error().Err(err).Msg("Failed to update test")
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to update test: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, testResp)
}

// DeleteTestHandler godoc
// @Summary Delete a test
// @Description Delete a test and its associated questions
// @Tags tests
// @Param id path int true "Test ID"
// @Success 204 "No Content"
// @Failure 400 {object} dto.ErrorResponse "Invalid ID format"
// @Failure 404 {object} dto.ErrorResponse "Test not found"
// @Failure 500 {object} dto.ErrorResponse "Internal server error"
// @Router /tests/{id} [delete]
func (ctrl *Controller) DeleteTestHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid test ID format"})
		return
	}

	if err := ctrl.testSvc.DeleteTest(uint(id)); err != nil {
		log.Error().Err(err).Msg("Failed to delete test")
		// Differentiate between Not Found and other errors
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to delete test: " + err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// AddQuestionToTestHandler godoc
// @Summary Add a question to an existing test
// @Description Creates a new question and associates it with the specified test
// @Tags tests
// @Accept json
// @Produce json
// @Param id path int true "Test ID to add question to"
// @Param question body dto.CreateQuestionRequest true "Question data (TestID in body will be ignored, OrderInTest should be unique for the test)"
// @Success 201 {object} dto.QuestionResponse
// @Failure 400 {object} dto.ErrorResponse "Invalid request body or Test ID"
// @Failure 404 {object} dto.ErrorResponse "Test not found"
// @Failure 500 {object} dto.ErrorResponse "Internal server error"
// @Router /tests/{id}/questions [post]
func (ctrl *Controller) AddQuestionToTestHandler(c *gin.Context) {
	testIdStr := c.Param("id")
	testID, err := strconv.ParseUint(testIdStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid Test ID format"})
		return
	}

	var req dto.CreateQuestionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warn().Err(err).Msg("Failed to bind CreateQuestionRequest for AddQuestionToTest")
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	// Validate OrderInTest
	if req.OrderInTest < 1 || req.OrderInTest > 8 {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "OrderInTest must be between 1 and 8"})
		return
	}
	// Additional validation: check if OrderInTest is unique for this test (service should ideally handle this)

	req.TestID = ptr(uint(testID)) // Ensure the question is associated with this test

	questionResp, err := ctrl.testSvc.AddQuestionToTest(uint(testID), req)
	if err != nil {
		log.Error().Err(err).Uint64("testID", testID).Msg("Failed to add question to test")
		// Service might return specific errors (e.g., test not found, validation error)
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to add question to test: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, questionResp)
}

// Helper function to get a pointer to a uint
func ptr[T any](v T) *T {
	return &v
}

// CreateQuestionHandler godoc
// @Summary Create a new question (standalone or for a test)
// @Description Add a new TOEIC writing question. If TestID is provided, it associates with that test.
// @Tags questions
// @Accept json
// @Produce json
// @Param question body dto.CreateQuestionRequest true "Question data"
// @Success 201 {object} dto.QuestionResponse
// @Failure 400 {object} dto.ErrorResponse "Invalid request body or invalid TestID"
// @Failure 500 {object} dto.ErrorResponse "Internal server error"
// @Router /questions [post]
func (ctrl *Controller) CreateQuestionHandler(c *gin.Context) {
	var req dto.CreateQuestionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warn().Err(err).Msg("Failed to bind CreateQuestionRequest")
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	// Validate specific fields based on type
	if req.Type == "sentence_picture" {
		if req.ImageURL == nil || req.GivenWord1 == nil || req.GivenWord2 == nil {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "For sentence_picture type, ImageURL, GivenWord1, and GivenWord2 are required."})
			return
		}
	}
	if req.TestID != nil && (req.OrderInTest < 1 || req.OrderInTest > 8) {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "If TestID is provided, OrderInTest must be between 1 and 8."})
		return
	}

	questionResp, err := ctrl.questionSvc.CreateQuestion(req)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create question controller")
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to create question: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, questionResp)
}

// GetQuestionHandler godoc
// @Summary Get a question by ID
// @Description Retrieve a specific TOEIC writing question
// @Tags questions
// @Produce json
// @Param id path int true "Question ID"
// @Success 200 {object} dto.QuestionResponse
// @Failure 400 {object} dto.ErrorResponse "Invalid ID format"
// @Failure 404 {object} dto.ErrorResponse "Question not found"
// @Failure 500 {object} dto.ErrorResponse "Internal server error"
// @Router /questions/{id} [get]
func (ctrl *Controller) GetQuestionHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid question ID format"})
		return
	}

	questionResp, err := ctrl.questionSvc.GetQuestion(uint(id))
	if err != nil {
		log.Error().Err(err).Uint64("id", id).Msg("Failed to get question")
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Question not found"})
		return
	}
	c.JSON(http.StatusOK, questionResp)
}

// GetAllQuestionsHandler godoc
// @Summary Get all questions, optionally filtered by test ID
// @Description Retrieve TOEIC writing questions. Use 'test_id' query param to filter by test.
// @Tags questions
// @Produce json
// @Param test_id query int false "Filter by Test ID"
// @Success 200 {array} dto.QuestionResponse
// @Failure 400 {object} dto.ErrorResponse "Invalid test_id format"
// @Failure 500 {object} dto.ErrorResponse "Internal server error"
// @Router /questions [get]
func (ctrl *Controller) GetAllQuestionsHandler(c *gin.Context) {
	var testID *uint
	testIDStr := c.Query("test_id")
	if testIDStr != "" {
		val, err := strconv.ParseUint(testIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid test_id format"})
			return
		}
		parsedTestID := uint(val)
		testID = &parsedTestID
	}

	questions, err := ctrl.questionSvc.GetAllQuestions(testID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get all questions")
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to retrieve questions"})
		return
	}
	c.JSON(http.StatusOK, questions)
}

// UpdateQuestionHandler godoc
// @Summary Update an existing question
// @Description Modify an existing TOEIC writing question
// @Tags questions
// @Accept json
// @Produce json
// @Param id path int true "Question ID"
// @Param question body dto.CreateQuestionRequest true "Updated question data"
// @Success 200 {object} dto.QuestionResponse
// @Failure 400 {object} dto.ErrorResponse "Invalid request body or ID"
// @Failure 404 {object} dto.ErrorResponse "Question not found"
// @Failure 500 {object} dto.ErrorResponse "Internal server error"
// @Router /questions/{id} [put]
func (ctrl *Controller) UpdateQuestionHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid question ID format"})
		return
	}

	var req dto.CreateQuestionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}
	// Validation for specific fields based on type
	if req.Type == "sentence_picture" {
		if req.ImageURL == nil || req.GivenWord1 == nil || req.GivenWord2 == nil {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "For sentence_picture type, ImageURL, GivenWord1, and GivenWord2 are required."})
			return
		}
	}
	if req.TestID != nil && (req.OrderInTest < 1 || req.OrderInTest > 8) {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "If TestID is provided, OrderInTest must be between 1 and 8."})
		return
	}

	questionResp, err := ctrl.questionSvc.UpdateQuestion(uint(id), req)
	if err != nil {
		log.Error().Err(err).Uint64("id", id).Msg("Failed to update question")
		// Differentiate between Not Found and other errors
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to update question: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, questionResp)
}

// DeleteQuestionHandler godoc
// @Summary Delete a question
// @Description Remove a question from the system
// @Tags questions
// @Param id path int true "Question ID"
// @Success 204 "No Content"
// @Failure 400 {object} dto.ErrorResponse "Invalid ID format"
// @Failure 404 {object} dto.ErrorResponse "Question not found"
// @Failure 500 {object} dto.ErrorResponse "Internal server error"
// @Router /questions/{id} [delete]
func (ctrl *Controller) DeleteQuestionHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid question ID format"})
		return
	}

	if err := ctrl.questionSvc.DeleteQuestion(uint(id)); err != nil {
		log.Error().Err(err).Uint64("id", id).Msg("Failed to delete question")
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to delete question: " + err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// SubmitAttemptHandler godoc
// @Summary Submit a new attempt for a question
// @Description User submits their answer to a question, it gets evaluated by AI
// @Tags attempts
// @Accept json
// @Produce json
// @Param attempt body dto.SubmitAttemptRequest true "Attempt data"
// @Success 201 {object} dto.AttemptResponse
// @Failure 400 {object} dto.ErrorResponse "Invalid request body or question not found"
// @Failure 500 {object} dto.ErrorResponse "Internal server error or AI service error"
// @Router /attempts [post]
func (ctrl *Controller) SubmitAttemptHandler(c *gin.Context) {
	var req dto.SubmitAttemptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warn().Err(err).Msg("Failed to bind SubmitAttemptRequest")
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	attemptResp, err := ctrl.attemptSvc.SubmitAttempt(req)
	if err != nil {
		log.Error().Err(err).Interface("request", req).Msg("Failed to submit attempt")
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to submit attempt: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, attemptResp)
}

// GetAllAttemptsHandler godoc
// @Summary Get all attempts (history), optionally filtered by user_id and/or test_id
// @Description Retrieve submitted attempts. Use 'user_id' and/or 'test_id' query params to filter.
// @Tags attempts
// @Produce json
// @Param user_id query int false "Filter attempts by User ID"
// @Param test_id query int false "Filter attempts by Test ID (shows attempts for questions within this test)"
// @Success 200 {array} dto.AttemptResponse
// @Failure 400 {object} dto.ErrorResponse "Invalid user_id or test_id format"
// @Failure 500 {object} dto.ErrorResponse "Internal server error"
// @Router /attempts [get]
func (ctrl *Controller) GetAllAttemptsHandler(c *gin.Context) {
	var userID *uint
	userIDStr := c.Query("user_id")
	if userIDStr != "" {
		val, err := strconv.ParseUint(userIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid user_id format"})
			return
		}
		parsedUserID := uint(val)
		userID = &parsedUserID
		log.Info().Uint("userID", *userID).Msg("Filtering attempts by UserID.")
	}

	var testID *uint
	testIDStr := c.Query("test_id")
	if testIDStr != "" {
		val, err := strconv.ParseUint(testIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid test_id format"})
			return
		}
		parsedTestID := uint(val)
		testID = &parsedTestID
		log.Info().Uint("testID", parsedTestID).Msg("Filtering attempts by TestID.")
	}

	if userID == nil && testID == nil {
		log.Info().Msg("Fetching all attempts (no user_id or test_id filter).")
	}

	// Truyền cả userID (có thể là nil) và testID (có thể là nil) vào service
	attempts, err := ctrl.attemptSvc.GetAllAttempts(userID, testID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get all attempts")
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to retrieve attempts history"})
		return
	}
	c.JSON(http.StatusOK, attempts)
}

// GetAttemptHandler godoc
// @Summary Get a specific attempt by ID
// @Description Retrieve details of a single submitted attempt
// @Tags attempts
// @Produce json
// @Param id path int true "Attempt ID"
// @Success 200 {object} dto.AttemptResponse
// @Failure 400 {object} dto.ErrorResponse "Invalid ID format"
// @Failure 404 {object} dto.ErrorResponse "Attempt not found"
// @Failure 500 {object} dto.ErrorResponse "Internal server error"
// @Router /attempts/{id} [get]
func (ctrl *Controller) GetAttemptHandler(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid attempt ID format"})
		return
	}

	attemptResp, err := ctrl.attemptSvc.GetAttempt(uint(id))
	if err != nil {
		log.Error().Err(err).Uint64("id", id).Msg("Failed to get attempt")
		c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: "Attempt not found"})
		return
	}
	c.JSON(http.StatusOK, attemptResp)
}

// SubmitFullTestHandler godoc
// @Summary Submit all answers for a specific test
// @Description User submits a collection of answers for all (or some) questions in a test.
// @Tags tests
// @Accept json
// @Produce json
// @Param id path int true "Test ID to submit answers for"
// @Param submission_data body dto.SubmitFullTestRequest true "User ID and list of answers (question_id, user_answer)"
// @Success 200 {object} dto.SubmitFullTestResponse "Successfully submitted with details of created attempts"
// @Failure 400 {object} dto.ErrorResponse "Invalid request body, Test ID, or invalid question IDs within submission"
// @Failure 404 {object} dto.ErrorResponse "Test not found"
// @Failure 500 {object} dto.ErrorResponse "Internal server error during submission processing"
// @Router /tests/{id}/submit [post]
func (ctrl *Controller) SubmitFullTestHandler(c *gin.Context) {
	testIdStr := c.Param("id")
	testID, err := strconv.ParseUint(testIdStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid Test ID format"})
		return
	}

	var req dto.SubmitFullTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Warn().Err(err).Msg("Failed to bind SubmitFullTestRequest")
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: err.Error()})
		return
	}

	// Validate if answers are provided
	if len(req.Answers) == 0 {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "No answers provided in the submission."})
		return
	}

	if req.UserID != nil {
		log.Info().Uint("userID", *req.UserID).Uint64("testID", testID).Msg("SubmitFullTest request received.")
	} else {
		log.Info().Uint64("testID", testID).Msg("SubmitFullTest request received without UserID.")
	}

	// Truyền GORM DB instance vào service method để nó có thể quản lý transaction
	submissionResp, err := ctrl.attemptSvc.SubmitFullTestAnswers(uint(testID), req, ctrl.db)
	if err != nil {
		log.Error().Err(err).Uint64("testID", testID).Msg("Error submitting full test answers")
		// Lỗi có thể là do test không tìm thấy, lỗi DB, v.v.
		// Service nên trả về các lỗi cụ thể hơn nếu có thể để controller map sang status code phù hợp
		if _, ok := err.(interface{ NotFound() }); ok { // Example check for a custom "not found" error
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to process full test submission: " + err.Error()})
		}
		return
	}

	// Kiểm tra xem có lỗi từng phần trong quá trình xử lý không
	if len(submissionResp.Errors) > 0 {
		log.Warn().Interface("partial_errors", submissionResp.Errors).Msg("Partial errors occurred during full test submission")
		// Trả về 207 Multi-Status nếu một số thành công, một số lỗi, hoặc 200 với danh sách lỗi
		// Để đơn giản, trả 200 OK nhưng bao gồm errors trong response body
	}

	c.JSON(http.StatusOK, submissionResp)
}

// GetTestAttemptHistoryHandler godoc
// @Summary Get user's attempt history for a specific test
// @Description Retrieves all questions of a test and the user's attempts for each question.
// @Tags tests
// @Produce json
// @Param id path int true "Test ID"
// @Param user_id query int false "User ID to filter history for. If not provided, might show for a default/anonymous user or be restricted."
// @Success 200 {object} dto.TestAttemptHistoryResponseDTO
// @Failure 400 {object} dto.ErrorResponse "Invalid Test ID or User ID format"
// @Failure 404 {object} dto.ErrorResponse "Test not found"
// @Failure 500 {object} dto.ErrorResponse "Internal server error"
// @Router /tests/{id}/history [get]
func (ctrl *Controller) GetTestAttemptHistoryHandler(c *gin.Context) {
	testIdStr := c.Param("id")
	testID, err := strconv.ParseUint(testIdStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid Test ID format"})
		return
	}

	var userID *uint
	userIDStr := c.Query("user_id")
	if userIDStr != "" {
		val, err := strconv.ParseUint(userIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid user_id format"})
			return
		}
		parsedUserID := uint(val)
		userID = &parsedUserID
	} else {
		// Xử lý trường hợp không có user_id:
		// 1. Trả lỗi nếu user_id là bắt buộc cho endpoint này.
		// 2. Hoặc gán một user_id mặc định (ví dụ: user khách).
		// Hiện tại, service sẽ xử lý userID là nil (có thể là lấy của user mặc định nếu service có logic đó, hoặc lấy tất cả attempts nếu không có user nào).
		// Tuy nhiên, cho "lịch sử CỦA TÔI", user_id thường là bắt buộc hoặc lấy từ context auth.
		// Vì đang hardcore, chúng ta sẽ cho phép nil và để service quyết định.
		log.Info().Uint64("testID", testID).Msg("GetTestAttemptHistory request without specific UserID.")
	}

	historyResp, err := ctrl.testSvc.GetTestAttemptHistory(uint(testID), userID)
	if err != nil {
		// Kiểm tra lỗi cụ thể hơn nếu service trả về
		if _, ok := err.(interface{ NotFound() }); ok { // Giả sử có custom error
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to retrieve test attempt history: " + err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, historyResp)
}

// GetCompletedTestsByUserHandler godoc
// @Summary Get all completed tests for a specific user
// @Description Retrieves all tests that a user has attempted
// @Tags tests
// @Produce json
// @Param user_id path int true "User ID"
// @Success 200 {array} dto.TestResponse
// @Failure 400 {object} dto.ErrorResponse "Invalid User ID format"
// @Failure 500 {object} dto.ErrorResponse "Internal server error"
// @Router /done/{user_id} [get]
func (ctrl *Controller) GetCompletedTestsByUserHandler(c *gin.Context) {
	userIDStr := c.Param("user_id")
	userID, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid User ID format"})
		return
	}

	parsedUserID := uint(userID)
	tests, err := ctrl.testSvc.GetCompletedTestsByUser(&parsedUserID)
	if err != nil {
		log.Error().Err(err).Uint("userID", parsedUserID).Msg("Failed to retrieve completed tests for user")
		c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to retrieve completed tests: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, tests)
}

// GetTestAttemptsByUserHandler godoc
// @Summary Get all attempts for a specific test by a user
// @Description Retrieves all attempts made by a user for a specific test
// @Tags tests
// @Produce json
// @Param test_id path int true "Test ID"
// @Param user_id path int true "User ID"
// @Success 200 {object} dto.TestAttemptsResponse
// @Failure 400 {object} dto.ErrorResponse "Invalid Test ID or User ID format"
// @Failure 404 {object} dto.ErrorResponse "Test not found"
// @Failure 500 {object} dto.ErrorResponse "Internal server error"
// @Router /result/{test_id}/{user_id} [get]
func (ctrl *Controller) GetTestAttemptsByUserHandler(c *gin.Context) {
	testIDStr := c.Param("test_id")
	testID, err := strconv.ParseUint(testIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid Test ID format"})
		return
	}

	userIDStr := c.Param("user_id")
	userID, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "Invalid User ID format"})
		return
	}

	parsedUserID := uint(userID)
	attempts, err := ctrl.testSvc.GetTestAttemptsByUser(uint(testID), &parsedUserID)
	if err != nil {
		// Check for specific error types
		if _, ok := err.(interface{ NotFound() }); ok {
			c.JSON(http.StatusNotFound, dto.ErrorResponse{Error: err.Error()})
		} else {
			log.Error().Err(err).Uint("testID", uint(testID)).Uint("userID", parsedUserID).Msg("Failed to retrieve test attempts")
			c.JSON(http.StatusInternalServerError, dto.ErrorResponse{Error: "Failed to retrieve test attempts: " + err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, attempts)
}

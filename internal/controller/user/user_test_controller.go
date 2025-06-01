package user

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/lshigami/Ringtails/internal/dto" // Corrected DTO path
	"github.com/lshigami/Ringtails/internal/service"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm" // For injecting DB into controller if service needs it directly (less ideal)
)

type UserTestController struct {
	userTestService       service.UserTestService
	testSubmissionService service.TestSubmissionService
	db                    *gorm.DB // Passed to submissionService's SubmitTest method
}

func NewUserTestController(uts service.UserTestService, tss service.TestSubmissionService, db *gorm.DB) *UserTestController {
	return &UserTestController{
		userTestService:       uts,
		testSubmissionService: tss,
		db:                    db,
	}
}

// GetAllTests godoc
// @Summary (User) List all available tests
// @Description Get a list of tests. If 'user_id' query param is provided, includes attempt status for that user.
// @Tags User - Tests & Attempts
// @Produce json
// @Param user_id query int false "Optional User ID to check attempt status against"
// @Success 200 {array} dto.TestSummaryDTO
// @Failure 400 {object} dto.ErrorResponse "Invalid User ID format"
// @Failure 500 {object} dto.ErrorResponse "Internal server error"
// @Router /tests [get]
func (c *UserTestController) GetAllTests(ctx *gin.Context) {
	var userID *uint
	userIDQueryStr := ctx.Query("user_id")
	if userIDQueryStr != "" {
		val, err := strconv.ParseUint(userIDQueryStr, 10, 32)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, dto.ErrorResponse{Message: "Invalid User ID format in query"})
			return
		}
		uID := uint(val)
		userID = &uID
		log.Info().Uint("userID", *userID).Msg("User GetAllTests: Fetching tests with attempt status for user.")
	} else {
		log.Info().Msg("User GetAllTests: Fetching all tests without user-specific attempt status.")
	}

	tests, err := c.userTestService.GetAllTests(userID) // Truyền userID vào service
	if err != nil {
		log.Error().Err(err).Msg("User GetAllTests: Service error")
		ctx.JSON(http.StatusInternalServerError, dto.ErrorResponse{Message: "Failed to retrieve tests", Details: []string{err.Error()}})
		return
	}
	ctx.JSON(http.StatusOK, tests)
}

// GetTestDetails godoc
// @Summary (User) Get details of a specific test
// @Description Get full details of a test, including all its questions, for a user to start an attempt.
// @Tags User - Tests & Attempts
// @Produce json
// @Param test_id path int true "Test ID"
// @Success 200 {object} dto.TestResponseDTO
// @Failure 400 {object} dto.ErrorResponse "Invalid Test ID format"
// @Failure 404 {object} dto.ErrorResponse "Test not found"
// @Failure 500 {object} dto.ErrorResponse "Internal server error"
// @Router /tests/{test_id} [get]
func (c *UserTestController) GetTestDetails(ctx *gin.Context) {
	testIDStr := ctx.Param("test_id")
	testID, err := strconv.ParseUint(testIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse{Message: "Invalid Test ID format"})
		return
	}
	testDetails, err := c.userTestService.GetTestDetails(uint(testID))
	if err != nil {
		// Here, you might want to check if err is a "not found" error from the service
		log.Warn().Err(err).Uint64("testID", testID).Msg("User GetTestDetails: Test not found or service error")
		ctx.JSON(http.StatusNotFound, dto.ErrorResponse{Message: err.Error()}) // Assuming service returns descriptive not found error
		return
	}
	ctx.JSON(http.StatusOK, testDetails)
}

// SubmitTestAttempt godoc
// @Summary (User) Submit answers for an entire test
// @Description User submits answers for questions in a specific test. AI scoring happens in the background.
// @Tags User - Tests & Attempts
// @Accept json
// @Produce json
// @Param test_id path int true "ID of the Test being attempted"
// @Param submission_data body dto.TestAttemptSubmitDTO true "User ID (optional for now) and list of answers"
// @Success 200 {object} dto.TestAttemptDetailDTO "Attempt submitted and processing started. Details might be partial until scoring completes."
// @Failure 400 {object} dto.ErrorResponse "Invalid input (e.g., bad Test ID, invalid answers format)"
// @Failure 404 {object} dto.ErrorResponse "Test not found"
// @Failure 500 {object} dto.ErrorResponse "Error processing submission"
// @Router /tests/{test_id}/attempts [post]
func (c *UserTestController) SubmitTestAttempt(ctx *gin.Context) {
	testIDStr := ctx.Param("test_id")
	testID, err := strconv.ParseUint(testIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse{Message: "Invalid Test ID format"})
		return
	}

	var req dto.TestAttemptSubmitDTO
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.Warn().Err(err).Msg("User SubmitTestAttempt: Failed to bind JSON")
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse{Message: "Invalid request body", Details: []string{err.Error()}})
		return
	}
	if len(req.Answers) == 0 {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse{Message: "Submission must contain at least one answer."})
		return
	}

	log.Info().Uint64("testID", testID).Interface("userID", req.UserID).Int("answerCount", len(req.Answers)).Msg("Received request to submit test attempt")

	// Pass the main DB instance from controller to service method for transaction management
	attemptDetail, err := c.testSubmissionService.SubmitTest(uint(testID), req)
	if err != nil {
		log.Error().Err(err).Uint64("testID", testID).Msg("User SubmitTestAttempt: Service error")
		// Differentiate errors: e.g., test not found vs. internal server error
		ctx.JSON(http.StatusInternalServerError, dto.ErrorResponse{Message: "Failed to submit test attempt", Details: []string{err.Error()}})
		return
	}
	// HTTP 202 Accepted could also be suitable if processing is truly async and client needs to poll
	// But for now, 200 OK with the created (or being processed) attempt details is fine.
	ctx.JSON(http.StatusOK, attemptDetail)
}

// GetUserTestAttempts godoc
// @Summary (User) Get all attempts by a user for a specific test
// @Description Retrieve a list of summary information for all attempts a user made on a test.
// @Tags User - Tests & Attempts
// @Produce json
// @Param test_id path int true "Test ID"
// @Param user_id query int false "User ID to filter attempts. (Temporary - will be from auth token)"
// @Success 200 {array} dto.TestAttemptSummaryDTO
// @Failure 400 {object} dto.ErrorResponse "Invalid ID format for Test ID or User ID"
// @Failure 500 {object} dto.ErrorResponse "Internal server error"
// @Router /tests/{test_id}/my-attempts [get]
func (c *UserTestController) GetUserTestAttempts(ctx *gin.Context) {
	testIDStr := ctx.Param("test_id")
	testID, err := strconv.ParseUint(testIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse{Message: "Invalid Test ID format"})
		return
	}

	var userID *uint
	userIDQueryStr := ctx.Query("user_id")
	if userIDQueryStr != "" {
		val, parseErr := strconv.ParseUint(userIDQueryStr, 10, 32)
		if parseErr != nil {
			ctx.JSON(http.StatusBadRequest, dto.ErrorResponse{Message: "Invalid User ID format in query"})
			return
		}
		uID := uint(val)
		userID = &uID
	} else {
		// If no user_id, service might fetch for an anonymous/default user, or all if allowed.
		// For "my-attempts", user context is usually implicit via auth.
		log.Info().Uint64("testID", testID).Msg("GetUserTestAttempts: No specific user_id query param, service will handle.")
	}

	attempts, err := c.testSubmissionService.GetUserAttemptsForTest(uint(testID), userID)
	if err != nil {
		log.Error().Err(err).Uint64("testID", testID).Interface("userID", userID).Msg("User GetUserTestAttempts: Service error")
		ctx.JSON(http.StatusInternalServerError, dto.ErrorResponse{Message: "Failed to retrieve user attempts for test", Details: []string{err.Error()}})
		return
	}
	ctx.JSON(http.StatusOK, attempts)
}

// GetSpecificTestAttemptDetails godoc
// @Summary (User) Get details of a specific test attempt
// @Description Retrieve full details of a single test attempt, including all answers, scores, and feedback.
// @Tags User - Tests & Attempts
// @Produce json
// @Param attempt_id path int true "Test Attempt ID"
// @Success 200 {object} dto.TestAttemptDetailDTO
// @Failure 400 {object} dto.ErrorResponse "Invalid Test Attempt ID format"
// @Failure 404 {object} dto.ErrorResponse "Test Attempt not found"
// @Failure 500 {object} dto.ErrorResponse "Internal server error"
// @Router /test-attempts/{attempt_id} [get]
func (c *UserTestController) GetSpecificTestAttemptDetails(ctx *gin.Context) {
	attemptIDStr := ctx.Param("attempt_id")
	attemptID, err := strconv.ParseUint(attemptIDStr, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse{Message: "Invalid Test Attempt ID format"})
		return
	}

	attemptDetails, err := c.testSubmissionService.GetTestAttemptDetails(uint(attemptID))
	if err != nil {
		log.Warn().Err(err).Uint64("attemptID", attemptID).Msg("User GetSpecificTestAttemptDetails: Attempt not found or service error")
		ctx.JSON(http.StatusNotFound, dto.ErrorResponse{Message: err.Error()}) // Assuming service returns descriptive not found error
		return
	}
	ctx.JSON(http.StatusOK, attemptDetails)
}

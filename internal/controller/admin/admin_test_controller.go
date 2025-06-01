package admin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lshigami/Ringtails/internal/dto" // Corrected DTO path
	"github.com/lshigami/Ringtails/internal/service"
	"github.com/rs/zerolog/log"
)

type AdminTestController struct {
	adminTestService service.AdminTestService
}

func NewAdminTestController(adminTestService service.AdminTestService) *AdminTestController {
	return &AdminTestController{adminTestService: adminTestService}
}

// CreateTest godoc
// @Summary (Admin) Create a new complete test
// @Description Admin creates a new test with exactly 8 questions. All questions must be provided.
// @Tags Admin - Tests
// @Accept json
// @Produce json
// @Param test_data body dto.TestCreateDTO true "Test creation data including all questions (must be 8 questions)"
// @Success 201 {object} dto.TestResponseDTO "Test created successfully"
// @Failure 400 {object} dto.ErrorResponse "Invalid input data (e.g., not 8 questions, missing fields)"
// @Failure 500 {object} dto.ErrorResponse "Internal server error"
// @Router /admin/tests [post]
func (c *AdminTestController) CreateTest(ctx *gin.Context) {
	var req dto.TestCreateDTO
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.Warn().Err(err).Msg("Admin CreateTest: Failed to bind JSON")
		errorDetails := []string{err.Error()}
		// You can add more detailed validation error parsing here if desired
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse{Message: "Invalid request body", Details: errorDetails})
		return
	}

	testResp, err := c.adminTestService.CreateTest(req)
	if err != nil {
		log.Error().Err(err).Interface("requestPayload", req).Msg("Admin CreateTest: Service error")
		// Determine if it's a client error (e.g. validation from service) or server error
		// For now, treating most service errors as potential client input issues or internal logic.
		// A more sophisticated error handling might differentiate.
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse{Message: "Failed to create test", Details: []string{err.Error()}})
		return
	}
	ctx.JSON(http.StatusCreated, testResp)
}

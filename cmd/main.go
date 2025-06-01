package main

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/lshigami/Ringtails/config"
	"github.com/lshigami/Ringtails/database"
	_ "github.com/lshigami/Ringtails/docs" // Swagger docs - auto-generated
	adminctrl "github.com/lshigami/Ringtails/internal/controller/admin"
	userctrl "github.com/lshigami/Ringtails/internal/controller/user"
	"github.com/lshigami/Ringtails/internal/logger" // Assuming logger.Init() is global or provide a logger instance
	"github.com/lshigami/Ringtails/internal/model"
	"github.com/lshigami/Ringtails/internal/repository"
	"github.com/lshigami/Ringtails/internal/service"
	"github.com/rs/zerolog/log" // Global zerolog instance
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/fx"
	"gorm.io/gorm"
)

// @title TOEIC Writing Practice API (Revised V1)
// @version 2.0
// @description API for TOEIC Writing practice with structured tests and AI feedback. Designed for full test submissions and history.
// @termsOfService http://swagger.io/terms/
// @contact.name API Support
// @contact.url http://example.com/support
// @contact.email support@example.com
// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
// @host localhost:8080
// @BasePath /api/v1
// @schemes http https
func main() {
	// Initialize global logger (if your logger.Init() does this)
	logger.Init() // Call this early

	app := fx.New(
		// Core Application Components
		fx.Provide(
			config.NewConfig,
			database.NewDatabase, // Provides *gorm.DB
			NewGinEngine,         // Provides *gin.Engine
		),

		// Repositories Layer
		fx.Provide(
			repository.NewTestRepository,
			repository.NewQuestionRepository,
			repository.NewTestAttemptRepository,
			repository.NewAnswerRepository,
		),

		// Services Layer
		fx.Provide(
			service.NewAdminTestService,
			func(testRepo repository.TestRepository, attemptRepo repository.TestAttemptRepository, sc service.ScoreConverterService) service.UserTestService {
				return service.NewUserTestService(testRepo, attemptRepo, sc)
			},
			service.NewGeminiLLMService, // Renamed Gemini service
			func(
				testRepo repository.TestRepository,
				questionRepo repository.QuestionRepository,
				testAttemptRepo repository.TestAttemptRepository,
				answerRepo repository.AnswerRepository,
				geminiService service.GeminiLLMService,
				sc service.ScoreConverterService, // Thêm ScoreConverterService
				db *gorm.DB,
			) service.TestSubmissionService {
				return service.NewTestSubmissionService(testRepo, questionRepo, testAttemptRepo, answerRepo, geminiService, sc, db)
			},
			service.NewScoreConverterService,
		),

		// API Controllers Layer
		fx.Provide(
			adminctrl.NewAdminTestController,
			// UserTestController needs *gorm.DB for TestSubmissionService's transaction handling
			func(uts service.UserTestService, tss service.TestSubmissionService, db *gorm.DB) *userctrl.UserTestController {
				return userctrl.NewUserTestController(uts, tss, db)
			},
		),

		// Invokers - Functions that are executed by Fx
		fx.Invoke(RegisterRoutesAndStartServer), // Combined registration and server start
		fx.Invoke(AutoMigrateDB),
	)

	// Start the application
	if err := app.Start(context.Background()); err != nil {
		log.Fatal().Err(err).Msg("Failed to start application")
	}

	// Wait for a shutdown signal
	<-app.Done()
	log.Info().Msg("Application shutting down gracefully...")
	// fx.Stop can be called here for graceful shutdown if hooks are complex
}

func NewGinEngine() *gin.Engine {
	// Set Gin mode based on an environment variable or config if desired
	// For development:
	gin.SetMode(gin.DebugMode)
	// For production:
	// gin.SetMode(gin.ReleaseMode)

	r := gin.New()

	// Custom logger using Zerolog for Gin (optional, Gin's default is also good)
	r.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		// Log with Zerolog
		log.Info().
			Str("client_ip", param.ClientIP).
			Str("method", param.Method).
			Str("path", param.Path).
			Int("status_code", param.StatusCode).
			Dur("latency", param.Latency).
			Str("user_agent", param.Request.UserAgent()).
			Str("error_message", param.ErrorMessage).
			Msg("gin_request")
		return "" // Returning empty string to avoid double logging if Gin's default logger is also active
	}))
	r.Use(gin.Recovery())

	// CORS Configuration
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"}, // Be more specific in production
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Swagger UI
	// URL: http://localhost:PORT/swagger/index.html
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return r
}

// RegisterRoutesAndStartServer configures API routes and manages server lifecycle.
func RegisterRoutesAndStartServer(
	lc fx.Lifecycle,
	router *gin.Engine,
	cfg *config.Config,
	adminTestCtrl *adminctrl.AdminTestController,
	userTestCtrl *userctrl.UserTestController,
) {
	// Admin Routes (prefixed with /api/v1/admin)
	adminAPIGroup := router.Group("/api/v1/admin")
	{
		testsAdminGroup := adminAPIGroup.Group("/tests")
		testsAdminGroup.POST("", adminTestCtrl.CreateTest)
		// Add more admin routes for tests here (e.g., update, delete test)
	}

	// User Routes (prefixed with /api/v1)
	userAPIGroup := router.Group("/api/v1")
	{
		// Test listing and details
		userAPIGroup.GET("/tests", userTestCtrl.GetAllTests)
		userAPIGroup.GET("/tests/:test_id", userTestCtrl.GetTestDetails)

		// Test Attempts
		userAPIGroup.POST("/tests/:test_id/attempts", userTestCtrl.SubmitTestAttempt)
		userAPIGroup.GET("/tests/:test_id/my-attempts", userTestCtrl.GetUserTestAttempts) // User ID from query/auth
		userAPIGroup.GET("/test-attempts/:attempt_id", userTestCtrl.GetSpecificTestAttemptDetails)
	}

	// HTTP Server Setup and Lifecycle
	server := &http.Server{
		Addr:    ":" + cfg.Server.Port,
		Handler: router,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Info().Msgf("TOEIC Writing API V1 server starting on port %s", cfg.Server.Port)
			log.Info().Msgf("Swagger UI available at http://localhost:%s/swagger/index.html", cfg.Server.Port)
			go func() {
				if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Fatal().Err(err).Msg("Server ListenAndServe failed")
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info().Msg("Server shutting down...")
			// Create a context with timeout for shutdown
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return server.Shutdown(shutdownCtx)
		},
	})
}

func AutoMigrateDB(db *gorm.DB) error {
	log.Info().Msg("Running database migrations for V1 models...")
	err := db.AutoMigrate(
		&model.Test{},
		&model.Question{},
		&model.TestAttempt{},
		&model.Answer{},
		// &model.User{}, // If you add a User model later
	)
	if err != nil {
		log.Error().Err(err).Msg("Database migration failed")
		return err
	}
	log.Info().Msg("Database V1 migration completed successfully.")
	return nil
}

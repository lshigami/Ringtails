package main

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/lshigami/Ringtails/config"
	"gorm.io/gorm"

	"github.com/lshigami/Ringtails/database"
	_ "github.com/lshigami/Ringtails/docs"
	"github.com/lshigami/Ringtails/internal/controller"
	"github.com/lshigami/Ringtails/internal/logger"
	"github.com/lshigami/Ringtails/internal/model"
	"github.com/lshigami/Ringtails/internal/repository"
	"github.com/lshigami/Ringtails/internal/service"
	"github.com/rs/zerolog/log"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"go.uber.org/fx"
)

// @title TOEIC Writing Practice API
// @version 1.1
// @description This is a server for a TOEIC Writing practice application with AI feedback. Supports structured tests and various question types including picture description.
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.url http://www.swagger.io/support
// @contact.email support@swagger.io

// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html

// @host localhost:8080
// @BasePath  /api/v1
// @schemes http https
func main() {

	app := fx.New(
		fx.Provide(
			config.NewConfig,
			database.NewDatabase,
			NewGinEngine,

			// Repositories
			repository.NewQuestionRepository,
			repository.NewAttemptRepository,
			repository.NewTestRepository,

			// Services
			func(testRepo repository.TestRepository, questionRepo repository.QuestionRepository, db *gorm.DB) service.TestService {
				return service.NewTestService(testRepo, questionRepo, db)
			},
			func(questionRepo repository.QuestionRepository, testRepo repository.TestRepository) service.QuestionService {
				return service.NewQuestionService(questionRepo, testRepo)
			},
			service.NewGeminiService,
			service.NewAttemptService,

			// Controllers
			func(qSvc service.QuestionService, attSvc service.AttemptService, tSvc service.TestService, db *gorm.DB) *controller.Controller {
				return controller.NewController(qSvc, attSvc, tSvc, db)
			},
		),
		fx.Invoke(RegisterRoutes),
		fx.Invoke(AutoMigrateDB),
	)

	if err := app.Start(context.Background()); err != nil {
		log.Fatal().Err(err).Msg("Failed to start application")
	}
	<-app.Done()
	log.Info().Msg("Application shutting down...")

}
func NewGinEngine() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// Configure CORS
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Add swagger route
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return r
}

func RegisterRoutes(
	lifecycle fx.Lifecycle,
	router *gin.Engine,
	cfg *config.Config,
	ctrl *controller.Controller,
) {
	ctrl.RegisterRoutes(router)
	logger.Init()

	server := &http.Server{
		Addr:    ":" + cfg.Server.Port,
		Handler: router,
	}

	lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			log.Info().Msgf("Starting server on port %s", cfg.Server.Port)
			log.Info().Msgf("Swagger UI available at http://localhost:%s/swagger/index.html", cfg.Server.Port)
			go func() {
				if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Fatal().Err(err).Msg("Failed to start server")
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info().Msg("Shutting down server")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			return server.Shutdown(shutdownCtx)
		},
	})
}

func AutoMigrateDB(db *gorm.DB) error {
	log.Info().Msg("Running database migrations...")
	err := db.AutoMigrate(
		&model.Test{}, // Add Test model
		&model.Question{},
		&model.Attempt{},
	)
	if err != nil {
		log.Error().Err(err).Msg("Failed to migrate database")
		return err
	}
	log.Info().Msg("Database migration completed successfully.")
	return nil
}

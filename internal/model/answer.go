package model

import (
	"time"

	"gorm.io/gorm"
)

type Answer struct {
	ID            uint           `gorm:"primarykey" json:"id"`
	TestAttemptID uint           `json:"test_attempt_id" gorm:"not null;index"`
	QuestionID    uint           `json:"question_id" gorm:"not null;index"`
	Question      Question       `json:"question,omitempty" gorm:"foreignKey:QuestionID"`
	UserAnswer    string         `json:"user_answer" gorm:"type:text;not null"`
	AIFeedback    string         `json:"ai_feedback,omitempty" gorm:"type:text"`
	AIScore       *float64       `json:"ai_score,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

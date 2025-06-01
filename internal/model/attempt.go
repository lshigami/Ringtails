package model

import (
	"time"

	"gorm.io/gorm"
)

type Attempt struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	QuestionID  uint           `json:"question_id" gorm:"not null"`
	Question    Question       `json:"question" gorm:"foreignKey:QuestionID"`
	UserAnswer  string         `json:"user_answer" gorm:"type:text;not null"`
	AIFeedback  string         `json:"ai_feedback" gorm:"type:text"`
	SubmittedAt time.Time      `json:"submitted_at" gorm:"autoCreateTime"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

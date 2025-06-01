package model

import (
	"time"

	"gorm.io/gorm"
)

type TestAttempt struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	TestID      uint           `json:"test_id" gorm:"not null;index"`
	Test        Test           `json:"test,omitempty" gorm:"foreignKey:TestID"`
	UserID      *uint          `json:"user_id,omitempty" gorm:"index"`
	SubmittedAt time.Time      `json:"submitted_at" gorm:"autoCreateTime"`
	TotalScore  *float64       `json:"total_score,omitempty"`
	Status      string         `json:"status" gorm:"default:'pending'"` // "pending", "scoring", "completed", "error", "completed_with_errors"
	Answers     []Answer       `json:"answers,omitempty" gorm:"foreignKey:TestAttemptID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

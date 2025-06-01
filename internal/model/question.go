package model

import (
	"time"

	"gorm.io/gorm"
)

type Question struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	TestID      uint           `json:"test_id" gorm:"not null;index"`
	Title       string         `json:"title" gorm:"not null"`
	Prompt      string         `json:"prompt" gorm:"type:text;not null"`
	Type        string         `json:"type" gorm:"not null"` // "sentence_picture", "email_response", "opinion_essay"
	OrderInTest int            `json:"order_in_test" gorm:"not null"`
	ImageURL    *string        `json:"image_url,omitempty"`
	GivenWord1  *string        `json:"given_word1,omitempty"`
	GivenWord2  *string        `json:"given_word2,omitempty"`
	MaxScore    float64        `json:"max_score,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

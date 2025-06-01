package model

import (
	"time"

	"gorm.io/gorm"
)

type Question struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	TestID      *uint          `json:"test_id,omitempty" gorm:"index"`
	Title       string         `json:"title" gorm:"not null"`
	Prompt      string         `json:"prompt" gorm:"type:text;not null"` // For email/essay content or general instructions for picture questions
	Type        string         `json:"type" gorm:"not null"`             // "sentence_picture", "email_response", "opinion_essay"
	OrderInTest int            `json:"order_in_test"`                    // 1-5 for picture, 6-7 for email, 8 for essay (0 if not in a test)
	ImageURL    *string        `json:"image_url,omitempty"`
	GivenWord1  *string        `json:"given_word1,omitempty"`
	GivenWord2  *string        `json:"given_word2,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

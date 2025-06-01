package model

import (
	"time"

	"gorm.io/gorm"
)

type Test struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	Title       string         `json:"title" gorm:"not null;uniqueIndex"`
	Description string         `json:"description,omitempty"`
	Questions   []Question     `json:"questions" gorm:"foreignKey:TestID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

package models

import "time"

type Deployment struct {
	ID          uint      `gorm:"primaryKey"`
	UserID      uint      `gorm:"index;not null"`
	ProjectName string    `gorm:"size:128;not null"`
	RawYAML     string    `gorm:"type:text;not null"`
	Status      string    `gorm:"size:32;not null;default:pending"`
	CreatedAt   time.Time `gorm:"not null"`
	UpdatedAt   time.Time `gorm:"not null"`
}

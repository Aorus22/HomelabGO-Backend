package models

import "time"

type CloudflareConfig struct {
	ID          uint      `gorm:"primaryKey"`
	UserID      uint      `gorm:"uniqueIndex;not null"`
	TunnelToken string    `gorm:"size:500"`
	CreatedAt   time.Time `gorm:"not null"`
	UpdatedAt   time.Time `gorm:"not null"`
}

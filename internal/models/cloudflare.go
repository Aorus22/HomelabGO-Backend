package models

import "time"

type CloudflareConfig struct {
	ID          uint      `gorm:"primaryKey"`
	UserID      uint      `gorm:"index;not null"` // Changed from uniqueIndex to allow multiple admin instances
	TunnelToken string    `gorm:"size:500"`
	ContainerID string    `gorm:"size:128"` // Docker container ID
	CreatedAt   time.Time `gorm:"not null"`
	UpdatedAt   time.Time `gorm:"not null"`
}

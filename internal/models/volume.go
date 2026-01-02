package models

import "time"

type Volume struct {
	ID         uint      `gorm:"primaryKey"`
	UserID     uint      `gorm:"index;not null"`
	VolumeName string    `gorm:"size:128;not null"`
	MountPath  string    `gorm:"size:255;not null"`
	CreatedAt  time.Time `gorm:"not null"`
	UpdatedAt  time.Time `gorm:"not null"`
}

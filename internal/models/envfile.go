package models

import "time"

type EnvFile struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	Content   string    `gorm:"type:text" json:"content"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt time.Time `gorm:"not null" json:"updated_at"`
}

// DeploymentEnvFile is a junction table for many-to-many relationship
type DeploymentEnvFile struct {
	DeploymentID uint `gorm:"primaryKey" json:"deployment_id"`
	EnvFileID    uint `gorm:"primaryKey" json:"env_file_id"`
}

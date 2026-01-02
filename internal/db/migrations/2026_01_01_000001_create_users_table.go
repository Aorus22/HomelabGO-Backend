package migrations

import (
	"gorm.io/gorm"
)

func init() {
	Register(Migration{
		ID: "2026_01_01_000001_create_users_table",
		Up: func(db *gorm.DB) error {
			return db.Exec(`
				CREATE TABLE IF NOT EXISTS users (
					id SERIAL PRIMARY KEY,
					username VARCHAR(64) UNIQUE NOT NULL,
					password_hash VARCHAR(255) NOT NULL,
					role VARCHAR(16) NOT NULL DEFAULT 'user',
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
				)
			`).Error
		},
		Down: func(db *gorm.DB) error {
			return db.Exec(`DROP TABLE IF EXISTS users`).Error
		},
	})
}

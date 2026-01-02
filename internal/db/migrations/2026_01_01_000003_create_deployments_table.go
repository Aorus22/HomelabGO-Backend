package migrations

import (
	"gorm.io/gorm"
)

func init() {
	Register(Migration{
		ID: "2026_01_01_000003_create_deployments_table",
		Up: func(db *gorm.DB) error {
			return db.Exec(`
				CREATE TABLE IF NOT EXISTS deployments (
					id SERIAL PRIMARY KEY,
					user_id INTEGER NOT NULL,
					project_name VARCHAR(128) NOT NULL,
					raw_yaml TEXT NOT NULL,
					status VARCHAR(32) NOT NULL DEFAULT 'pending',
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
				);
				CREATE INDEX IF NOT EXISTS idx_deployments_user_id ON deployments(user_id);
			`).Error
		},
		Down: func(db *gorm.DB) error {
			return db.Exec(`DROP TABLE IF EXISTS deployments`).Error
		},
	})
}

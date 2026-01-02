package migrations

import (
	"gorm.io/gorm"
)

func init() {
	Register(Migration{
		ID: "2026_01_01_000002_create_volumes_table",
		Up: func(db *gorm.DB) error {
			return db.Exec(`
				CREATE TABLE IF NOT EXISTS volumes (
					id SERIAL PRIMARY KEY,
					user_id INTEGER NOT NULL,
					volume_name VARCHAR(128) NOT NULL,
					mount_path VARCHAR(255) NOT NULL,
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
				);
				CREATE INDEX IF NOT EXISTS idx_volumes_user_id ON volumes(user_id);
			`).Error
		},
		Down: func(db *gorm.DB) error {
			return db.Exec(`DROP TABLE IF EXISTS volumes`).Error
		},
	})
}

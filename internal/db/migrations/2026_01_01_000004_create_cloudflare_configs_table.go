package migrations

import (
	"gorm.io/gorm"
)

func init() {
	Register(Migration{
		ID: "2026_01_01_000004_create_cloudflare_configs_table",
		Up: func(db *gorm.DB) error {
			return db.Exec(`
				CREATE TABLE IF NOT EXISTS cloudflare_configs (
					id SERIAL PRIMARY KEY,
					user_id INTEGER UNIQUE NOT NULL,
					tunnel_token VARCHAR(500),
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
				);
				CREATE INDEX IF NOT EXISTS idx_cloudflare_configs_user_id ON cloudflare_configs(user_id);
			`).Error
		},
		Down: func(db *gorm.DB) error {
			return db.Exec(`DROP TABLE IF EXISTS cloudflare_configs`).Error
		},
	})
}

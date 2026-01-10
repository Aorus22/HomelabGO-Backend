package migrations

import (
	"gorm.io/gorm"
)

func init() {
	Register(Migration{
		ID: "2026_01_03_000005_create_envfiles_table",
		Up: func(db *gorm.DB) error {
			// Create env_files table
			if err := db.Exec(`
				CREATE TABLE IF NOT EXISTS env_files (
					id SERIAL PRIMARY KEY,
					user_id INTEGER NOT NULL,
					name VARCHAR(128) NOT NULL,
					content TEXT,
					created_at TIMESTAMP NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
					FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
				)
			`).Error; err != nil {
				return err
			}

			// Create index on user_id
			if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_env_files_user_id ON env_files(user_id)`).Error; err != nil {
				return err
			}

			// Create junction table for deployment-envfile many-to-many
			if err := db.Exec(`
				CREATE TABLE IF NOT EXISTS deployment_env_files (
					deployment_id INTEGER NOT NULL,
					env_file_id INTEGER NOT NULL,
					PRIMARY KEY (deployment_id, env_file_id),
					FOREIGN KEY (deployment_id) REFERENCES deployments(id) ON DELETE CASCADE,
					FOREIGN KEY (env_file_id) REFERENCES env_files(id) ON DELETE CASCADE
				)
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Down: func(db *gorm.DB) error {
			if err := db.Exec(`DROP TABLE IF EXISTS deployment_env_files`).Error; err != nil {
				return err
			}
			return db.Exec(`DROP TABLE IF EXISTS env_files`).Error
		},
	})
}

package migrations

import (
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm"
)

type Migration struct {
	ID   string
	Up   func(db *gorm.DB) error
	Down func(db *gorm.DB) error
}

type MigrationHistory struct {
	ID        uint      `gorm:"primaryKey"`
	Migration string    `gorm:"size:255;uniqueIndex;not null"`
	Batch     int       `gorm:"not null"`
	AppliedAt time.Time `gorm:"not null"`
}

func (MigrationHistory) TableName() string {
	return "migrations"
}

type Migrator struct {
	db         *gorm.DB
	migrations []Migration
}

func NewMigrator(db *gorm.DB) *Migrator {
	return &Migrator{
		db:         db,
		migrations: registeredMigrations,
	}
}

func (m *Migrator) EnsureMigrationsTable() error {
	return m.db.AutoMigrate(&MigrationHistory{})
}

func (m *Migrator) GetAppliedMigrations() (map[string]bool, error) {
	var history []MigrationHistory
	if err := m.db.Find(&history).Error; err != nil {
		return nil, err
	}

	applied := make(map[string]bool)
	for _, h := range history {
		applied[h.Migration] = true
	}
	return applied, nil
}

func (m *Migrator) GetCurrentBatch() (int, error) {
	var maxBatch int
	err := m.db.Model(&MigrationHistory{}).Select("COALESCE(MAX(batch), 0)").Scan(&maxBatch).Error
	return maxBatch, err
}

func (m *Migrator) Migrate() error {
	if err := m.EnsureMigrationsTable(); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	applied, err := m.GetAppliedMigrations()
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	batch, err := m.GetCurrentBatch()
	if err != nil {
		return fmt.Errorf("failed to get current batch: %w", err)
	}
	batch++

	sort.Slice(m.migrations, func(i, j int) bool {
		return m.migrations[i].ID < m.migrations[j].ID
	})

	pending := 0
	for _, migration := range m.migrations {
		if applied[migration.ID] {
			continue
		}

		fmt.Printf("Migrating: %s\n", migration.ID)

		if err := migration.Up(m.db); err != nil {
			return fmt.Errorf("migration %s failed: %w", migration.ID, err)
		}

		history := MigrationHistory{
			Migration: migration.ID,
			Batch:     batch,
			AppliedAt: time.Now(),
		}
		if err := m.db.Create(&history).Error; err != nil {
			return fmt.Errorf("failed to record migration %s: %w", migration.ID, err)
		}

		fmt.Printf("Migrated:  %s\n", migration.ID)
		pending++
	}

	if pending == 0 {
		fmt.Println("Nothing to migrate.")
	} else {
		fmt.Printf("Migrated %d migration(s).\n", pending)
	}

	return nil
}

func (m *Migrator) Rollback() error {
	if err := m.EnsureMigrationsTable(); err != nil {
		return fmt.Errorf("failed to ensure migrations table: %w", err)
	}

	batch, err := m.GetCurrentBatch()
	if err != nil {
		return fmt.Errorf("failed to get current batch: %w", err)
	}

	if batch == 0 {
		fmt.Println("Nothing to rollback.")
		return nil
	}

	var history []MigrationHistory
	if err := m.db.Where("batch = ?", batch).Order("id DESC").Find(&history).Error; err != nil {
		return fmt.Errorf("failed to get batch migrations: %w", err)
	}

	migrationMap := make(map[string]Migration)
	for _, migration := range m.migrations {
		migrationMap[migration.ID] = migration
	}

	for _, h := range history {
		migration, exists := migrationMap[h.Migration]
		if !exists {
			fmt.Printf("Warning: Migration %s not found in code, skipping\n", h.Migration)
			continue
		}

		fmt.Printf("Rolling back: %s\n", h.Migration)

		if migration.Down != nil {
			if err := migration.Down(m.db); err != nil {
				return fmt.Errorf("rollback %s failed: %w", h.Migration, err)
			}
		}

		if err := m.db.Delete(&h).Error; err != nil {
			return fmt.Errorf("failed to remove migration record %s: %w", h.Migration, err)
		}

		fmt.Printf("Rolled back: %s\n", h.Migration)
	}

	return nil
}

func (m *Migrator) Status() error {
	if err := m.EnsureMigrationsTable(); err != nil {
		return fmt.Errorf("failed to ensure migrations table: %w", err)
	}

	applied, err := m.GetAppliedMigrations()
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	fmt.Println("\n+------+------------------------------------------+--------+")
	fmt.Println("| Ran? | Migration                                | Batch  |")
	fmt.Println("+------+------------------------------------------+--------+")

	var history []MigrationHistory
	m.db.Find(&history)
	batchMap := make(map[string]int)
	for _, h := range history {
		batchMap[h.Migration] = h.Batch
	}

	sort.Slice(m.migrations, func(i, j int) bool {
		return m.migrations[i].ID < m.migrations[j].ID
	})

	for _, migration := range m.migrations {
		ran := "No"
		batch := "-"
		if applied[migration.ID] {
			ran = "Yes"
			if b, ok := batchMap[migration.ID]; ok {
				batch = fmt.Sprintf("%d", b)
			}
		}
		fmt.Printf("| %-4s | %-40s | %-6s |\n", ran, migration.ID, batch)
	}

	fmt.Println("+------+------------------------------------------+--------+")

	return nil
}

var registeredMigrations []Migration

func Register(m Migration) {
	registeredMigrations = append(registeredMigrations, m)
}

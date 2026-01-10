package main

import (
	"fmt"
	"log"
	"os"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"homelabgo/internal/config"
	"homelabgo/internal/models"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Default admin credentials
	username := "admin"
	password := "admin123"

	// Allow override from args
	if len(os.Args) > 1 {
		username = os.Args[1]
	}
	if len(os.Args) > 2 {
		password = os.Args[2]
	}

	// Check if admin already exists
	var existing models.User
	if err := db.Where("username = ?", username).First(&existing).Error; err == nil {
		// Update existing user to admin
		existing.Role = "admin"
		if err := db.Save(&existing).Error; err != nil {
			log.Fatalf("Failed to update user to admin: %v", err)
		}
		fmt.Printf("✅ User '%s' updated to admin role\n", username)
		return
	}

	// Create new admin user
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	admin := models.User{
		Username:     username,
		PasswordHash: string(passwordHash),
		Role:         "admin",
	}

	if err := db.Create(&admin).Error; err != nil {
		log.Fatalf("Failed to create admin user: %v", err)
	}

	fmt.Printf("✅ Admin user created successfully!\n")
	fmt.Printf("   Username: %s\n", username)
	fmt.Printf("   Password: %s\n", password)
	fmt.Printf("\n⚠️  Please change the password after first login!\n")
}

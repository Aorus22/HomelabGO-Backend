package main

import (
	"log"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"homelabgo/internal/config"
	"homelabgo/internal/db"
	"homelabgo/internal/docker"
	httpapi "homelabgo/internal/http"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	database, err := db.Connect(cfg)
	if err != nil {
		log.Fatalf("database error: %v", err)
	}

	dockerClient, err := docker.NewClient()
	if err != nil {
		log.Fatalf("docker error: %v", err)
	}
	defer dockerClient.Close()

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	httpapi.RegisterRoutes(router, httpapi.Dependencies{
		DB:     database,
		Config: cfg,
		Docker: dockerClient,
	})

	addr := ":" + cfg.HTTPPort
	if err := router.Run(addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

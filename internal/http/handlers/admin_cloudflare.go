package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"homelabgo/internal/docker"
	"homelabgo/internal/models"
)

type AdminCloudflareHandler struct {
	db     *gorm.DB
	docker *docker.Client
}

func NewAdminCloudflareHandler(db *gorm.DB, docker *docker.Client) *AdminCloudflareHandler {
	return &AdminCloudflareHandler{db: db, docker: docker}
}

// List returns all admin cloudflare instances
func (h *AdminCloudflareHandler) List(c *gin.Context) {
	var configs []models.CloudflareConfig
	// Admin configs have user_id = 0
	if err := h.db.Where("user_id = ?", 0).Find(&configs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch configs"})
		return
	}

	result := make([]gin.H, len(configs))
	for i, cfg := range configs {
		status := "stopped"
		if cfg.ContainerID != "" {
			// Check if container is running
			if ctr, err := h.docker.GetContainerByIDNoAuth(c.Request.Context(), cfg.ContainerID); err == nil && ctr != nil {
				status = ctr.State
			}
		}
		maskedToken := cfg.TunnelToken
		if len(maskedToken) > 10 {
			maskedToken = maskedToken[:10] + "..."
		}
		result[i] = gin.H{
			"id":           cfg.ID,
			"token":        maskedToken,
			"container_id": cfg.ContainerID,
			"status":       status,
		}
	}

	c.JSON(http.StatusOK, result)
}

// Create creates a new admin cloudflare instance
func (h *AdminCloudflareHandler) Create(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
		return
	}

	config := models.CloudflareConfig{
		UserID:      0, // Admin-owned
		TunnelToken: req.Token,
	}

	if err := h.db.Create(&config).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create config"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":      config.ID,
		"message": "cloudflare instance created",
	})
}

// Start starts an admin cloudflare instance (always with --network host)
func (h *AdminCloudflareHandler) Start(c *gin.Context) {
	idParam := c.Param("id")
	configID, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config id"})
		return
	}

	var config models.CloudflareConfig
	if err := h.db.Where("id = ? AND user_id = ?", configID, 0).First(&config).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "config not found"})
		return
	}

	if config.ContainerID != "" {
		// Check if already running
		if ctr, err := h.docker.GetContainerByIDNoAuth(c.Request.Context(), config.ContainerID); err == nil && ctr != nil && ctr.State == "running" {
			c.JSON(http.StatusOK, gin.H{"message": "already running", "container_id": config.ContainerID})
			return
		}
		// Remove old container
		_ = h.docker.RemoveContainer(c.Request.Context(), config.ContainerID)
	}

	// Create and start container with --network host
	ctx := c.Request.Context()
	containerName := fmt.Sprintf("admin_cloudflared_%d", config.ID)
	containerID, err := h.createHostNetworkContainer(ctx, containerName, config.TunnelToken)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start cloudflared: " + err.Error()})
		return
	}

	config.ContainerID = containerID
	h.db.Save(&config)

	c.JSON(http.StatusOK, gin.H{
		"message":      "cloudflared started with host network",
		"container_id": containerID,
	})
}

// Stop stops an admin cloudflare instance
func (h *AdminCloudflareHandler) Stop(c *gin.Context) {
	idParam := c.Param("id")
	configID, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config id"})
		return
	}

	var config models.CloudflareConfig
	if err := h.db.Where("id = ? AND user_id = ?", configID, 0).First(&config).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "config not found"})
		return
	}

	if config.ContainerID != "" {
		_ = h.docker.StopContainerByID(c.Request.Context(), config.ContainerID)
		_ = h.docker.RemoveContainer(c.Request.Context(), config.ContainerID)
		config.ContainerID = ""
		h.db.Save(&config)
	}

	c.JSON(http.StatusOK, gin.H{"message": "cloudflared stopped"})
}

// Delete deletes an admin cloudflare instance
func (h *AdminCloudflareHandler) Delete(c *gin.Context) {
	idParam := c.Param("id")
	configID, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config id"})
		return
	}

	var config models.CloudflareConfig
	if err := h.db.Where("id = ? AND user_id = ?", configID, 0).First(&config).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "config not found"})
		return
	}

	// Stop and remove container if running
	if config.ContainerID != "" {
		_ = h.docker.StopContainerByID(c.Request.Context(), config.ContainerID)
		_ = h.docker.RemoveContainer(c.Request.Context(), config.ContainerID)
	}

	if err := h.db.Delete(&config).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete config"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "cloudflare instance deleted"})
}

// createHostNetworkContainer creates a cloudflared container with host network mode
func (h *AdminCloudflareHandler) createHostNetworkContainer(ctx context.Context, name, token string) (string, error) {
	api := h.docker.GetAPI()

	// Pull image first
	reader, _ := api.ImagePull(ctx, "cloudflare/cloudflared:latest", image.PullOptions{})
	if reader != nil {
		defer reader.Close()
	}

	// Remove existing container with same name
	_ = api.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})

	// Create container with host network
	resp, err := api.ContainerCreate(ctx,
		&container.Config{
			Image: "cloudflare/cloudflared:latest",
			Cmd:   []string{"tunnel", "--no-autoupdate", "run", "--token", token},
			Labels: map[string]string{
				"managed_by": "homelabgo",
				"type":       "admin_cloudflared",
			},
		},
		&container.HostConfig{
			NetworkMode: "host",
			RestartPolicy: container.RestartPolicy{
				Name: container.RestartPolicyAlways,
			},
		},
		nil,
		nil,
		name,
	)
	if err != nil {
		return "", err
	}

	if err := api.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", err
	}

	return resp.ID, nil
}

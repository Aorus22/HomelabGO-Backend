package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"homelabgo/internal/docker"
	"homelabgo/internal/httputil"
	"homelabgo/internal/models"
)

type CloudflareHandler struct {
	db     *gorm.DB
	docker *docker.Client
}

type updateCloudflareRequest struct {
	TunnelToken string `json:"tunnel_token" binding:"required"`
}

type cloudflareConfigResponse struct {
	Configured  bool   `json:"configured"`
	TunnelToken string `json:"tunnel_token,omitempty"` // Masked
}

func NewCloudflareHandler(db *gorm.DB, dockerClient *docker.Client) *CloudflareHandler {
	return &CloudflareHandler{
		db:     db,
		docker: dockerClient,
	}
}

func maskToken(token string) string {
	if len(token) <= 12 {
		return "****"
	}
	return token[:4] + strings.Repeat("*", len(token)-8) + token[len(token)-4:]
}

func (h *CloudflareHandler) GetConfig(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var config models.CloudflareConfig
	if err := h.db.Where("user_id = ?", userID).First(&config).Error; err != nil {
		c.JSON(http.StatusOK, cloudflareConfigResponse{
			Configured: false,
		})
		return
	}

	c.JSON(http.StatusOK, cloudflareConfigResponse{
		Configured:  true,
		TunnelToken: maskToken(config.TunnelToken),
	})
}

func (h *CloudflareHandler) UpdateConfig(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req updateCloudflareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	if len(req.TunnelToken) < 20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tunnel token"})
		return
	}

	var config models.CloudflareConfig
	if err := h.db.Where("user_id = ?", userID).First(&config).Error; err != nil {
		config = models.CloudflareConfig{
			UserID:      userID,
			TunnelToken: req.TunnelToken,
		}
		if err := h.db.Create(&config).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save config"})
			return
		}
	} else {
		config.TunnelToken = req.TunnelToken
		if err := h.db.Save(&config).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update config"})
			return
		}
	}

	if err := h.docker.DeployCloudflared(c.Request.Context(), userID, req.TunnelToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to deploy cloudflared: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "cloudflared deployed successfully",
		"tunnel_token": maskToken(req.TunnelToken),
	})
}

func (h *CloudflareHandler) GetStatus(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	status, err := h.docker.GetCloudflaredStatus(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get status"})
		return
	}

	c.JSON(http.StatusOK, status)
}

func (h *CloudflareHandler) GetLogs(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	tail := c.DefaultQuery("tail", "100")

	logs, err := h.docker.GetCloudflaredLogs(c.Request.Context(), userID, tail)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

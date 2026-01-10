package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"homelabgo/internal/httputil"
	"homelabgo/internal/models"
)

type EnvFileHandler struct {
	db *gorm.DB
}

type createEnvFileRequest struct {
	Name    string `json:"name" binding:"required"`
	Content string `json:"content"`
}

type updateEnvFileRequest struct {
	Name    string `json:"name,omitempty"`
	Content string `json:"content,omitempty"`
}

type envFileResponse struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type envFileListResponse struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func NewEnvFileHandler(db *gorm.DB) *EnvFileHandler {
	return &EnvFileHandler{db: db}
}

func (h *EnvFileHandler) List(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var envFiles []models.EnvFile
	if err := h.db.Where("user_id = ?", userID).Order("name ASC").Find(&envFiles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch env files"})
		return
	}

	response := make([]envFileListResponse, len(envFiles))
	for i, e := range envFiles {
		response[i] = envFileListResponse{
			ID:        e.ID,
			Name:      e.Name,
			CreatedAt: e.CreatedAt.Format(time.RFC3339),
			UpdatedAt: e.UpdatedAt.Format(time.RFC3339),
		}
	}

	c.JSON(http.StatusOK, response)
}

func (h *EnvFileHandler) Create(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req createEnvFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	// Check for duplicate name
	var existing models.EnvFile
	if err := h.db.Where("user_id = ? AND name = ?", userID, req.Name).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "env file with this name already exists"})
		return
	}

	envFile := models.EnvFile{
		UserID:  userID,
		Name:    req.Name,
		Content: req.Content,
	}

	if err := h.db.Create(&envFile).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create env file"})
		return
	}

	c.JSON(http.StatusCreated, envFileResponse{
		ID:        envFile.ID,
		Name:      envFile.Name,
		Content:   envFile.Content,
		CreatedAt: envFile.CreatedAt.Format(time.RFC3339),
		UpdatedAt: envFile.UpdatedAt.Format(time.RFC3339),
	})
}

func (h *EnvFileHandler) Get(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	envFileID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid env file id"})
		return
	}

	var envFile models.EnvFile
	if err := h.db.Where("id = ? AND user_id = ?", envFileID, userID).First(&envFile).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "env file not found"})
		return
	}

	c.JSON(http.StatusOK, envFileResponse{
		ID:        envFile.ID,
		Name:      envFile.Name,
		Content:   envFile.Content,
		CreatedAt: envFile.CreatedAt.Format(time.RFC3339),
		UpdatedAt: envFile.UpdatedAt.Format(time.RFC3339),
	})
}

func (h *EnvFileHandler) Update(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	envFileID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid env file id"})
		return
	}

	var envFile models.EnvFile
	if err := h.db.Where("id = ? AND user_id = ?", envFileID, userID).First(&envFile).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "env file not found"})
		return
	}

	var req updateEnvFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	if req.Name != "" && req.Name != envFile.Name {
		// Check for duplicate name
		var existing models.EnvFile
		if err := h.db.Where("user_id = ? AND name = ? AND id != ?", userID, req.Name, envFileID).First(&existing).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "env file with this name already exists"})
			return
		}
		envFile.Name = req.Name
	}

	if req.Content != "" {
		envFile.Content = req.Content
	}

	if err := h.db.Save(&envFile).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update env file"})
		return
	}

	c.JSON(http.StatusOK, envFileResponse{
		ID:        envFile.ID,
		Name:      envFile.Name,
		Content:   envFile.Content,
		CreatedAt: envFile.CreatedAt.Format(time.RFC3339),
		UpdatedAt: envFile.UpdatedAt.Format(time.RFC3339),
	})
}

func (h *EnvFileHandler) Delete(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	envFileID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid env file id"})
		return
	}

	var envFile models.EnvFile
	if err := h.db.Where("id = ? AND user_id = ?", envFileID, userID).First(&envFile).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "env file not found"})
		return
	}

	// Delete from junction table first
	h.db.Where("env_file_id = ?", envFileID).Delete(&models.DeploymentEnvFile{})

	if err := h.db.Delete(&envFile).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete env file"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "env file deleted"})
}

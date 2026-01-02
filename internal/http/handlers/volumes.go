package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"homelabgo/internal/docker"
	"homelabgo/internal/httputil"
	"homelabgo/internal/models"
)

type VolumeHandler struct {
	db     *gorm.DB
	docker *docker.Client
}

type createVolumeRequest struct {
	Name string `json:"name" binding:"required"`
}

type volumeResponse struct {
	ID         uint      `json:"id"`
	Name       string    `json:"name"`
	VolumeName string    `json:"volume_name"`
	MountPath  string    `json:"mount_path"`
	CreatedAt  time.Time `json:"created_at"`
}

func NewVolumeHandler(db *gorm.DB, dockerClient *docker.Client) *VolumeHandler {
	return &VolumeHandler{
		db:     db,
		docker: dockerClient,
	}
}

func (h *VolumeHandler) List(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var volumes []models.Volume
	if err := h.db.Where("user_id = ?", userID).Find(&volumes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch volumes"})
		return
	}

	response := make([]volumeResponse, len(volumes))
	for i, v := range volumes {
		response[i] = volumeResponse{
			ID:         v.ID,
			Name:       v.VolumeName,
			VolumeName: v.VolumeName,
			MountPath:  v.MountPath,
			CreatedAt:  v.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, response)
}

func (h *VolumeHandler) Create(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req createVolumeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	// Check if volume with same name already exists for this user
	var existing models.Volume
	if err := h.db.Where("user_id = ? AND volume_name = ?", userID, req.Name).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "volume with this name already exists"})
		return
	}

	volInfo, err := h.docker.CreateVolume(c.Request.Context(), userID, req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create docker volume: " + err.Error()})
		return
	}

	volume := models.Volume{
		UserID:     userID,
		VolumeName: req.Name,
		MountPath:  volInfo.MountPath,
	}

	if err := h.db.Create(&volume).Error; err != nil {
		// Try to cleanup Docker volume
		_ = h.docker.DeleteVolume(c.Request.Context(), userID, req.Name)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save volume"})
		return
	}

	c.JSON(http.StatusCreated, volumeResponse{
		ID:         volume.ID,
		Name:       volume.VolumeName,
		VolumeName: volume.VolumeName,
		MountPath:  volume.MountPath,
		CreatedAt:  volume.CreatedAt,
	})
}

func (h *VolumeHandler) Delete(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	volumeID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid volume id"})
		return
	}

	var volume models.Volume
	if err := h.db.Where("id = ? AND user_id = ?", volumeID, userID).First(&volume).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "volume not found"})
		return
	}

	if err := h.docker.DeleteVolume(c.Request.Context(), userID, volume.VolumeName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete docker volume: " + err.Error()})
		return
	}

	if err := h.db.Delete(&volume).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete volume record"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "volume deleted"})
}

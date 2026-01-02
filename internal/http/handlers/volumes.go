package handlers

import (
	"fmt"
	"io"
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

func (h *VolumeHandler) Download(c *gin.Context) {
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

	// Get volume info to get mount path
	volInfo, err := h.docker.GetVolume(c.Request.Context(), userID, volume.VolumeName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get volume info"})
		return
	}

	// Create tar.gz from volume mount path
	tarData, err := h.docker.DownloadVolumeAsTarGz(c.Request.Context(), userID, volume.VolumeName, volInfo.MountPath)
	if err != nil {
		fmt.Printf("Error downloading volume: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create archive: " + err.Error()})
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.tar.gz", volume.VolumeName))
	c.Data(http.StatusOK, "application/gzip", tarData)
}

func (h *VolumeHandler) Upload(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	volumeName := c.PostForm("name")
	if volumeName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "volume name is required"})
		return
	}

	// Check if volume with same name already exists
	var existing models.Volume
	if err := h.db.Where("user_id = ? AND volume_name = ?", userID, volumeName).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "volume with this name already exists"})
		return
	}

	file, _, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	defer file.Close()

	// Read tar.gz file
	tarData, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	// Create volume and extract tar.gz
	volInfo, err := h.docker.CreateVolumeFromTarGz(c.Request.Context(), userID, volumeName, tarData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create volume: " + err.Error()})
		return
	}

	volume := models.Volume{
		UserID:     userID,
		VolumeName: volumeName,
		MountPath:  volInfo.MountPath,
	}

	if err := h.db.Create(&volume).Error; err != nil {
		_ = h.docker.DeleteVolume(c.Request.Context(), userID, volumeName)
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

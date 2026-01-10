package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"homelabgo/internal/docker"
	"homelabgo/internal/models"
)

type AdminHandler struct {
	db     *gorm.DB
	docker *docker.Client
}

func NewAdminHandler(db *gorm.DB, docker *docker.Client) *AdminHandler {
	return &AdminHandler{db: db, docker: docker}
}

type userListItem struct {
	ID              uint   `json:"id"`
	Username        string `json:"username"`
	Role            string `json:"role"`
	DeploymentCount int64  `json:"deployment_count"`
	VolumeCount     int64  `json:"volume_count"`
	EnvFileCount    int64  `json:"env_file_count"`
}

// ListUsers returns all users with resource counts
func (h *AdminHandler) ListUsers(c *gin.Context) {
	var users []models.User
	if err := h.db.Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch users"})
		return
	}

	result := make([]userListItem, len(users))
	for i, u := range users {
		var deploymentCount, volumeCount, envFileCount int64
		h.db.Model(&models.Deployment{}).Where("user_id = ?", u.ID).Count(&deploymentCount)
		h.db.Model(&models.Volume{}).Where("user_id = ?", u.ID).Count(&volumeCount)
		h.db.Model(&models.EnvFile{}).Where("user_id = ?", u.ID).Count(&envFileCount)

		result[i] = userListItem{
			ID:              u.ID,
			Username:        u.Username,
			Role:            u.Role,
			DeploymentCount: deploymentCount,
			VolumeCount:     volumeCount,
			EnvFileCount:    envFileCount,
		}
	}

	c.JSON(http.StatusOK, result)
}

type userDetail struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// GetUser returns a single user's details
func (h *AdminHandler) GetUser(c *gin.Context) {
	idParam := c.Param("id")
	userID, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var user models.User
	if err := h.db.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, userDetail{
		ID:       user.ID,
		Username: user.Username,
		Role:     user.Role,
	})
}

// GetUserDeployments returns all deployments for a specific user
func (h *AdminHandler) GetUserDeployments(c *gin.Context) {
	idParam := c.Param("id")
	userID, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var deployments []models.Deployment
	if err := h.db.Where("user_id = ?", userID).Find(&deployments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch deployments"})
		return
	}

	result := make([]gin.H, len(deployments))
	for i, d := range deployments {
		// Get container count
		containers, _ := h.docker.GetProjectContainers(c.Request.Context(), uint(userID), d.ID)
		result[i] = gin.H{
			"id":              d.ID,
			"project_name":    d.ProjectName,
			"status":          d.Status,
			"container_count": len(containers),
			"created_at":      d.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, result)
}

// GetUserVolumes returns all volumes for a specific user
func (h *AdminHandler) GetUserVolumes(c *gin.Context) {
	idParam := c.Param("id")
	userID, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var volumes []models.Volume
	if err := h.db.Where("user_id = ?", userID).Find(&volumes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch volumes"})
		return
	}

	result := make([]gin.H, len(volumes))
	for i, v := range volumes {
		result[i] = gin.H{
			"id":          v.ID,
			"name":        v.VolumeName,
			"volume_name": v.VolumeName,
			"created_at":  v.CreatedAt,
		}
	}

	c.JSON(http.StatusOK, result)
}

// GetUserEnvFiles returns all env files for a specific user (names only, no content)
func (h *AdminHandler) GetUserEnvFiles(c *gin.Context) {
	idParam := c.Param("id")
	userID, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var envFiles []models.EnvFile
	if err := h.db.Where("user_id = ?", userID).Find(&envFiles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch env files"})
		return
	}

	result := make([]gin.H, len(envFiles))
	for i, e := range envFiles {
		result[i] = gin.H{
			"id":         e.ID,
			"name":       e.Name,
			"created_at": e.CreatedAt,
			"updated_at": e.UpdatedAt,
		}
	}

	c.JSON(http.StatusOK, result)
}

// ListAllContainers returns all containers managed by the backend with owner info
func (h *AdminHandler) ListAllContainers(c *gin.Context) {
	// Get all users
	var users []models.User
	if err := h.db.Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch users"})
		return
	}

	userMap := make(map[uint]string)
	for _, u := range users {
		userMap[u.ID] = u.Username
	}

	// Get all containers for all users
	allContainers := make([]gin.H, 0)
	for _, u := range users {
		containers, err := h.docker.ListContainers(c.Request.Context(), u.ID)
		if err != nil {
			continue
		}

		for _, ctr := range containers {
			allContainers = append(allContainers, gin.H{
				"id":           ctr.ID[:12],
				"name":         ctr.Name,
				"image":        ctr.Image,
				"status":       ctr.Status,
				"state":        ctr.State,
				"owner_id":     u.ID,
				"owner_name":   u.Username,
				"project_name": ctr.ProjectName,
				"service_name": ctr.ServiceName,
			})
		}
	}

	c.JSON(http.StatusOK, allContainers)
}

// GetDeploymentContainers returns containers for a specific deployment
func (h *AdminHandler) GetDeploymentContainers(c *gin.Context) {
	userIDParam := c.Param("id")
	deploymentIDParam := c.Param("did")

	userID, err := strconv.ParseUint(userIDParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	deploymentID, err := strconv.ParseUint(deploymentIDParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment id"})
		return
	}

	containers, err := h.docker.GetProjectContainers(c.Request.Context(), uint(userID), uint(deploymentID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch containers"})
		return
	}

	result := make([]gin.H, len(containers))
	for i, ctr := range containers {
		result[i] = gin.H{
			"id":     ctr.ID[:12],
			"name":   ctr.Names[0],
			"image":  ctr.Image,
			"status": ctr.Status,
			"state":  ctr.State,
		}
	}

	c.JSON(http.StatusOK, result)
}

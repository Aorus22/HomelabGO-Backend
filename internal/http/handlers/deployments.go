package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"homelabgo/internal/docker"
	"homelabgo/internal/httputil"
	"homelabgo/internal/models"
)

type DeploymentHandler struct {
	db     *gorm.DB
	docker *docker.Client
}

type createDeploymentRequest struct {
	ProjectName string `json:"project_name" binding:"required"`
	RawYAML     string `json:"raw_yaml" binding:"required"`
}

type updateDeploymentRequest struct {
	ProjectName string `json:"project_name,omitempty"`
	RawYAML     string `json:"raw_yaml,omitempty"`
}

type validateRequest struct {
	RawYAML string `json:"raw_yaml" binding:"required"`
}

type deploymentResponse struct {
	ID          uint   `json:"id"`
	ProjectName string `json:"project_name"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type deploymentDetailResponse struct {
	ID          uint   `json:"id"`
	ProjectName string `json:"project_name"`
	RawYAML     string `json:"raw_yaml"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func NewDeploymentHandler(db *gorm.DB, dockerClient *docker.Client) *DeploymentHandler {
	return &DeploymentHandler{
		db:     db,
		docker: dockerClient,
	}
}

func (h *DeploymentHandler) List(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var deployments []models.Deployment
	if err := h.db.Where("user_id = ?", userID).Find(&deployments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch deployments"})
		return
	}

	response := make([]deploymentResponse, len(deployments))
	for i, d := range deployments {
		response[i] = deploymentResponse{
			ID:          d.ID,
			ProjectName: d.ProjectName,
			Status:      d.Status,
			CreatedAt:   d.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:   d.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	c.JSON(http.StatusOK, response)
}

func (h *DeploymentHandler) Create(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req createDeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	if err := docker.ValidateComposeYAML(req.RawYAML); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid YAML: " + err.Error()})
		return
	}

	var existing models.Deployment
	if err := h.db.Where("user_id = ? AND project_name = ?", userID, req.ProjectName).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "project with this name already exists"})
		return
	}

	deployment := models.Deployment{
		UserID:      userID,
		ProjectName: req.ProjectName,
		RawYAML:     req.RawYAML,
		Status:      "pending",
	}

	if err := h.db.Create(&deployment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create deployment"})
		return
	}

	c.JSON(http.StatusCreated, deploymentResponse{
		ID:          deployment.ID,
		ProjectName: deployment.ProjectName,
		Status:      deployment.Status,
		CreatedAt:   deployment.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   deployment.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

func (h *DeploymentHandler) Get(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	deploymentID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment id"})
		return
	}

	var deployment models.Deployment
	if err := h.db.Where("id = ? AND user_id = ?", deploymentID, userID).First(&deployment).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return
	}

	c.JSON(http.StatusOK, deploymentDetailResponse{
		ID:          deployment.ID,
		ProjectName: deployment.ProjectName,
		RawYAML:     deployment.RawYAML,
		Status:      deployment.Status,
		CreatedAt:   deployment.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   deployment.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

func (h *DeploymentHandler) Update(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	deploymentID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment id"})
		return
	}

	var deployment models.Deployment
	if err := h.db.Where("id = ? AND user_id = ?", deploymentID, userID).First(&deployment).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return
	}

	var req updateDeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	if req.RawYAML != "" {
		if err := docker.ValidateComposeYAML(req.RawYAML); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid YAML: " + err.Error()})
			return
		}
		deployment.RawYAML = req.RawYAML
	}

	if req.ProjectName != "" {
		deployment.ProjectName = req.ProjectName
	}

	if err := h.db.Save(&deployment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update deployment"})
		return
	}

	c.JSON(http.StatusOK, deploymentDetailResponse{
		ID:          deployment.ID,
		ProjectName: deployment.ProjectName,
		RawYAML:     deployment.RawYAML,
		Status:      deployment.Status,
		CreatedAt:   deployment.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   deployment.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

func (h *DeploymentHandler) Delete(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	deploymentID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment id"})
		return
	}

	var deployment models.Deployment
	if err := h.db.Where("id = ? AND user_id = ?", deploymentID, userID).First(&deployment).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return
	}

	if err := h.docker.RemoveProjectContainers(c.Request.Context(), userID, uint(deploymentID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove containers: " + err.Error()})
		return
	}

	if err := h.db.Delete(&deployment).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete deployment"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deployment deleted"})
}

func (h *DeploymentHandler) Validate(c *gin.Context) {
	var req validateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	if err := docker.ValidateComposeYAML(req.RawYAML); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"valid": false,
			"error": err.Error(),
		})
		return
	}

	compose, _ := docker.ParseComposeYAML(req.RawYAML)
	services := make([]string, 0, len(compose.Services))
	for name := range compose.Services {
		services = append(services, name)
	}

	c.JSON(http.StatusOK, gin.H{
		"valid":    true,
		"services": services,
	})
}

func (h *DeploymentHandler) Deploy(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	deploymentID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment id"})
		return
	}

	var deployment models.Deployment
	if err := h.db.Where("id = ? AND user_id = ?", deploymentID, userID).First(&deployment).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return
	}

	deployment.Status = "deploying"
	h.db.Save(&deployment)

	_ = h.docker.RemoveProjectContainers(c.Request.Context(), userID, uint(deploymentID))

	deployed, err := h.docker.DeployCompose(c.Request.Context(), userID, uint(deploymentID), deployment.ProjectName, deployment.RawYAML)
	if err != nil {
		deployment.Status = "failed"
		h.db.Save(&deployment)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "deployment failed: " + err.Error()})
		return
	}

	deployment.Status = "running"
	h.db.Save(&deployment)

	containers := make([]gin.H, len(deployed))
	for i, d := range deployed {
		containers[i] = gin.H{
			"id":           d.ID[:12],
			"name":         d.Name,
			"service_name": d.ServiceName,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "deployment successful",
		"status":     "running",
		"containers": containers,
	})
}

func (h *DeploymentHandler) Stop(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	deploymentID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment id"})
		return
	}

	var deployment models.Deployment
	if err := h.db.Where("id = ? AND user_id = ?", deploymentID, userID).First(&deployment).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return
	}

	if err := h.docker.StopProjectContainers(c.Request.Context(), userID, uint(deploymentID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to stop containers: " + err.Error()})
		return
	}

	deployment.Status = "stopped"
	h.db.Save(&deployment)

	c.JSON(http.StatusOK, gin.H{"message": "deployment stopped", "status": "stopped"})
}

func (h *DeploymentHandler) Start(c *gin.Context) {

	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	deploymentID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment id"})
		return
	}

	var deployment models.Deployment
	if err := h.db.Where("id = ? AND user_id = ?", deploymentID, userID).First(&deployment).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found"})
		return
	}

	deployed, redeployed, err := h.docker.SmartStartCompose(c.Request.Context(), userID, uint(deploymentID), deployment.ProjectName, deployment.RawYAML)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start containers: " + err.Error()})
		return
	}

	deployment.Status = "running"
	h.db.Save(&deployment)

	msg := "deployment started"
	if redeployed {
		msg = "deployment updated and started"
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    msg,
		"status":     "running",
		"containers": deployed, // might be nil if only started, but frontend re-fetches list usually
	})
}

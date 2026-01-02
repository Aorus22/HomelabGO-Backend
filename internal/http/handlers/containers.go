package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"homelabgo/internal/docker"
	"homelabgo/internal/httputil"
)

type ContainerHandler struct {
	db     *gorm.DB
	docker *docker.Client
}

func NewContainerHandler(db *gorm.DB, dockerClient *docker.Client) *ContainerHandler {
	return &ContainerHandler{
		db:     db,
		docker: dockerClient,
	}
}

func (h *ContainerHandler) List(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	containers, err := h.docker.ListContainersByUser(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list containers: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, containers)
}

func (h *ContainerHandler) Get(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	containerID := c.Param("id")
	if containerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "container id required"})
		return
	}

	container, err := h.docker.GetContainerByID(c.Request.Context(), containerID, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, container)
}

func (h *ContainerHandler) Start(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	containerID := c.Param("id")
	if containerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "container id required"})
		return
	}

	if err := h.docker.StartContainer(c.Request.Context(), containerID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start container: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "container started"})
}

func (h *ContainerHandler) Stop(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	containerID := c.Param("id")
	if containerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "container id required"})
		return
	}

	if err := h.docker.StopContainer(c.Request.Context(), containerID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to stop container: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "container stopped"})
}

func (h *ContainerHandler) Restart(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	containerID := c.Param("id")
	if containerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "container id required"})
		return
	}

	if err := h.docker.RestartContainer(c.Request.Context(), containerID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to restart container: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "container restarted"})
}

func (h *ContainerHandler) Logs(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	containerID := c.Param("id")
	if containerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "container id required"})
		return
	}

	tail := c.DefaultQuery("tail", "100")
	since := c.Query("since")

	logs, err := h.docker.GetContainerLogs(c.Request.Context(), containerID, userID, tail, since)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get logs: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

func (h *ContainerHandler) Pull(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	containerID := c.Param("id")
	if containerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "container id required"})
		return
	}

	if err := h.docker.PullContainerImage(c.Request.Context(), containerID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to pull image: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "image pulled successfully"})
}

func (h *ContainerHandler) Recreate(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	containerID := c.Param("id")
	if containerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "container id required"})
		return
	}

	if err := h.docker.RecreateContainer(c.Request.Context(), containerID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to recreate container: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "container recreated successfully"})
}

func (h *ContainerHandler) Stats(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	containerID := c.Param("id")
	if containerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "container id required"})
		return
	}

	stats, err := h.docker.GetContainerStats(c.Request.Context(), containerID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get stats: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

func (h *ContainerHandler) Mounts(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	containerID := c.Param("id")
	if containerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "container id required"})
		return
	}

	mounts, err := h.docker.GetContainerMounts(c.Request.Context(), containerID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get mounts: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, mounts)
}

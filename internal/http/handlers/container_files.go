package handlers

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"homelabgo/internal/docker"
	"homelabgo/internal/httputil"
)

type ContainerFileHandler struct {
	docker *docker.Client
}

func NewContainerFileHandler(docker *docker.Client) *ContainerFileHandler {
	return &ContainerFileHandler{
		docker: docker,
	}
}

func (h *ContainerFileHandler) ListFiles(c *gin.Context) {
	userID := httputil.GetUserID(c)
	containerID := c.Param("id")
	path := c.Query("path")

	if path == "" {
		path = "/"
	}

	files, err := h.docker.ListContainerFiles(c.Request.Context(), containerID, userID, path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, files)
}

func (h *ContainerFileHandler) GetFileContent(c *gin.Context) {
	userID := httputil.GetUserID(c)
	containerID := c.Param("id")
	path := c.Query("path")

	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	content, err := h.docker.ReadContainerFile(c.Request.Context(), containerID, userID, path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"path":    path,
		"content": string(content),
	})
}

func (h *ContainerFileHandler) DownloadFile(c *gin.Context) {
	userID := httputil.GetUserID(c)
	containerID := c.Param("id")
	path := c.Query("path")

	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	content, err := h.docker.ReadContainerFile(c.Request.Context(), containerID, userID, path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Disposition", "attachment; filename="+path)
	c.Data(http.StatusOK, "application/octet-stream", content)
}

func (h *ContainerFileHandler) UploadFile(c *gin.Context) {
	userID := httputil.GetUserID(c)
	containerID := c.Param("id")
	path := c.Query("path")

	if path == "" {
		path = "/"
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	defer file.Close()

	destPath := path + "/" + header.Filename
	if path[len(path)-1] == '/' {
		destPath = path + header.Filename
	}

	content, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	err = h.docker.WriteContainerFile(c.Request.Context(), containerID, userID, destPath, content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "path": destPath})
}

func (h *ContainerFileHandler) SaveFileContent(c *gin.Context) {
	userID := httputil.GetUserID(c)
	containerID := c.Param("id")
	path := c.Query("path")

	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	err := h.docker.WriteContainerFile(c.Request.Context(), containerID, userID, path, []byte(req.Content))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *ContainerFileHandler) CreateDirectory(c *gin.Context) {
	userID := httputil.GetUserID(c)
	containerID := c.Param("id")
	path := c.Query("path")

	var req struct {
		Path string `json:"path"`
	}

	if path == "" {
		if err := c.ShouldBindJSON(&req); err == nil {
			path = req.Path
		}
	}

	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	err := h.docker.CreateContainerDir(c.Request.Context(), containerID, userID, path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "path": path})
}

func (h *ContainerFileHandler) DeleteFile(c *gin.Context) {
	userID := httputil.GetUserID(c)
	containerID := c.Param("id")
	path := c.Query("path")

	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	err := h.docker.DeleteContainerFile(c.Request.Context(), containerID, userID, path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *ContainerFileHandler) RenameFile(c *gin.Context) {
	userID := httputil.GetUserID(c)
	containerID := c.Param("id")

	var req struct {
		OldPath string `json:"old_path"`
		NewPath string `json:"new_path"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "old_path and new_path are required"})
		return
	}

	if req.OldPath == "" || req.NewPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "old_path and new_path are required"})
		return
	}

	err := h.docker.RenameContainerFile(c.Request.Context(), containerID, userID, req.OldPath, req.NewPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "path": req.NewPath})
}

func (h *ContainerFileHandler) CopyFile(c *gin.Context) {
	userID := httputil.GetUserID(c)
	containerID := c.Param("id")

	var req struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source and destination are required"})
		return
	}

	if req.Source == "" || req.Destination == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source and destination are required"})
		return
	}

	err := h.docker.CopyContainerFile(c.Request.Context(), containerID, userID, req.Source, req.Destination)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "destination": req.Destination})
}

func (h *ContainerFileHandler) MoveFile(c *gin.Context) {
	userID := httputil.GetUserID(c)
	containerID := c.Param("id")

	var req struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source and destination are required"})
		return
	}

	if req.Source == "" || req.Destination == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source and destination are required"})
		return
	}

	err := h.docker.MoveContainerFile(c.Request.Context(), containerID, userID, req.Source, req.Destination)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok", "destination": req.Destination})
}

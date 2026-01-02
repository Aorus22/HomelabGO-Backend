package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"homelabgo/internal/httputil"
)

type FileHandler struct {
	db             *gorm.DB
	dataVolumePath string
}

type fileInfo struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	IsDir     bool      `json:"is_dir"`
	Size      int64     `json:"size"`
	ModTime   time.Time `json:"mod_time"`
	Extension string    `json:"extension,omitempty"`
}

func NewFileHandler(db *gorm.DB, dataVolumePath string) *FileHandler {
	return &FileHandler{
		db:             db,
		dataVolumePath: dataVolumePath,
	}
}

func (h *FileHandler) getUserBasePath(userID uint) string {
	return filepath.Join(h.dataVolumePath, fmt.Sprintf("user_%d", userID))
}

func (h *FileHandler) validatePath(userID uint, requestedPath string) (string, error) {
	basePath := h.getUserBasePath(userID)

	cleanPath := filepath.Clean(requestedPath)
	if strings.HasPrefix(cleanPath, "..") || strings.Contains(cleanPath, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid path: directory traversal not allowed")
	}

	fullPath := filepath.Join(basePath, cleanPath)

	if !strings.HasPrefix(fullPath, basePath) {
		return "", fmt.Errorf("invalid path: outside user directory")
	}

	return fullPath, nil
}

func (h *FileHandler) ensureUserDir(userID uint) error {
	basePath := h.getUserBasePath(userID)
	return os.MkdirAll(basePath, 0755)
}

func (h *FileHandler) List(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	if err := h.ensureUserDir(userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to access storage"})
		return
	}

	basePath := h.getUserBasePath(userID)
	files, err := h.listDirectory(basePath, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list files"})
		return
	}

	c.JSON(http.StatusOK, files)
}

func (h *FileHandler) GetFile(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	requestedPath := c.Param("path")
	if requestedPath == "" {
		requestedPath = "/"
	}

	fullPath, err := h.validatePath(userID, requestedPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to access file"})
		return
	}

	if info.IsDir() {
		files, err := h.listDirectory(fullPath, requestedPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list directory"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"type":  "directory",
			"path":  requestedPath,
			"files": files,
		})
		return
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"type":    "file",
		"path":    requestedPath,
		"name":    info.Name(),
		"size":    info.Size(),
		"content": string(content),
	})
}

func (h *FileHandler) SaveFile(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	requestedPath := c.Param("path")
	if requestedPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}

	fullPath, err := h.validatePath(userID, requestedPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create directory"})
		return
	}

	if err := os.WriteFile(fullPath, []byte(req.Content), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "file saved", "path": requestedPath})
}

func (h *FileHandler) DeleteFile(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	requestedPath := c.Param("path")
	if requestedPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}

	fullPath, err := h.validatePath(userID, requestedPath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Don't allow deleting user's root directory
	basePath := h.getUserBasePath(userID)
	if fullPath == basePath {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete root directory"})
		return
	}

	if err := os.RemoveAll(fullPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted", "path": requestedPath})
}

func (h *FileHandler) Upload(c *gin.Context) {
	userID := httputil.GetUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	if err := h.ensureUserDir(userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to access storage"})
		return
	}

	// Get target directory from form
	targetDir := c.PostForm("path")
	if targetDir == "" {
		targetDir = "/"
	}

	fullDir, err := h.validatePath(userID, targetDir)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Ensure target directory exists
	if err := os.MkdirAll(fullDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create directory"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file required"})
		return
	}
	defer file.Close()

	// Create destination file
	destPath := filepath.Join(fullDir, header.Filename)
	destFile, err := os.Create(destPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create file"})
		return
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, file); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "file uploaded",
		"filename": header.Filename,
		"path":     filepath.Join(targetDir, header.Filename),
	})
}

func (h *FileHandler) listDirectory(dirPath string, relativePath string) ([]fileInfo, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	files := make([]fileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		filePath := relativePath
		if filePath == "" || filePath == "/" {
			filePath = "/" + entry.Name()
		} else {
			filePath = filepath.Join(relativePath, entry.Name())
		}

		fi := fileInfo{
			Name:    entry.Name(),
			Path:    filePath,
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}

		if !entry.IsDir() {
			fi.Extension = strings.TrimPrefix(filepath.Ext(entry.Name()), ".")
		}

		files = append(files, fi)
	}

	return files, nil
}

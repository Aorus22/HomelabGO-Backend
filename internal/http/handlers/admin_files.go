package handlers

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type AdminFilesHandler struct {
	dataVolumePath string
}

func NewAdminFilesHandler(dataVolumePath string) *AdminFilesHandler {
	return &AdminFilesHandler{
		dataVolumePath: dataVolumePath,
	}
}

type adminFileInfo struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	IsDir   bool      `json:"is_dir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// ListFiles lists files in the system for admin (Webmin-style)
func (h *AdminFilesHandler) ListFiles(c *gin.Context) {
	requestedPath := c.Query("path")
	if requestedPath == "" {
		// Default to system root or user home
		if runtime.GOOS == "windows" {
			requestedPath = "C:\\"
		} else {
			requestedPath = "/"
		}
	}

	// Clean path
	fullPath := filepath.Clean(requestedPath)

	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		c.JSON(http.StatusOK, gin.H{
			"path":  requestedPath,
			"files": []adminFileInfo{},
			"error": "Path does not exist",
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to access path: " + err.Error()})
		return
	}

	if !info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is not a directory"})
		return
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read directory: " + err.Error()})
		return
	}

	files := make([]adminFileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Correctly join paths
		filePath := filepath.Join(fullPath, entry.Name())

		files = append(files, adminFileInfo{
			Name:    entry.Name(),
			Path:    filePath,
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"path":  fullPath,
		"files": files,
	})
}

// GetFile reads file content
func (h *AdminFilesHandler) GetFile(c *gin.Context) {
	requestedPath := c.Query("path")
	if requestedPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}

	fullPath := filepath.Clean(requestedPath)

	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to access file: " + err.Error()})
		return
	}

	if info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is a directory"})
		return
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"path":     fullPath,
		"content":  string(content),
		"size":     info.Size(),
		"mod_time": info.ModTime(),
	})
}

// SaveFile saves file content
func (h *AdminFilesHandler) SaveFile(c *gin.Context) {
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	if req.Path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}

	fullPath := filepath.Clean(req.Path)

	err := os.WriteFile(fullPath, []byte(req.Content), 0644)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "file saved"})
}

// HostTerminal provides WebSocket terminal to host shell
func (h *AdminFilesHandler) HostTerminal(c *gin.Context) {
	var upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// Determine shell
	shell := "/bin/bash"
	if runtime.GOOS == "windows" {
		shell = "cmd.exe"
	}

	// Use generic command execution
	// Note: For a real terminal experience, pty is required.
	// This simple implementation uses pipes which has limitations (no tab completion, simple prompts).
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	cmd := exec.CommandContext(ctx, shell)
	// We do NOT set cmd.Dir to dataVolumePath, let it start in default (usually home or working dir)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Error stdin: "+err.Error()))
		return
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Error stdout: "+err.Error()))
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Error stderr: "+err.Error()))
		return
	}

	if err := cmd.Start(); err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Error starting shell: "+err.Error()))
		return
	}

	// Read stdout with buffer to capture prompts without newlines
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				conn.WriteMessage(websocket.TextMessage, buf[:n])
			}
			if err != nil {
				break
			}
		}
	}()

	// Read stderr
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				conn.WriteMessage(websocket.TextMessage, buf[:n])
			}
			if err != nil {
				break
			}
		}
	}()

	// Read from websocket and write to stdin
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}
		// Append newline if likely missing, though usually terminals send Enter as is.
		// For cmd.exe, ensure \r\n
		input := string(message)
		if !strings.HasSuffix(input, "\n") {
			input += "\n"
		}
		stdin.Write([]byte(input))
	}

	stdin.Close()
	cmd.Wait()
}

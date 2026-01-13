package handlers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
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
	Owner   string    `json:"owner"`
	Group   string    `json:"group"`
	Perm    string    `json:"perm"`
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

		owner, group := resolveOwnerGroup(info)
		files = append(files, adminFileInfo{
			Name:    entry.Name(),
			Path:    filePath,
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			Owner:   owner,
			Group:   group,
			Perm:    formatPerm(info.Mode().Perm()),
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

func (h *AdminFilesHandler) CreateFile(c *gin.Context) {
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	if strings.TrimSpace(req.Path) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}

	fullPath := filepath.Clean(req.Path)
	if _, err := os.Stat(fullPath); err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file already exists"})
		return
	}

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create directory: " + err.Error()})
		return
	}

	if err := os.WriteFile(fullPath, []byte(req.Content), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create file: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "file created", "path": fullPath})
}

func (h *AdminFilesHandler) CreateDirectory(c *gin.Context) {
	var req struct {
		Path string `json:"path"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	if strings.TrimSpace(req.Path) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}

	fullPath := filepath.Clean(req.Path)
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create directory: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "directory created", "path": fullPath})
}

func (h *AdminFilesHandler) DeleteFile(c *gin.Context) {
	requestedPath := c.Query("path")
	if strings.TrimSpace(requestedPath) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path required"})
		return
	}

	fullPath := filepath.Clean(requestedPath)
	if fullPath == string(os.PathSeparator) || fullPath == "." {
		c.JSON(http.StatusBadRequest, gin.H{"error": "refusing to delete root"})
		return
	}
	if runtime.GOOS == "windows" && strings.HasPrefix(fullPath, "\\\\") == false {
		if matched, _ := regexp.MatchString("^[a-zA-Z]:\\\\?$", fullPath); matched {
			c.JSON(http.StatusBadRequest, gin.H{"error": "refusing to delete root"})
			return
		}
	}

	if err := os.RemoveAll(fullPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted", "path": fullPath})
}

func (h *AdminFilesHandler) RenameFile(c *gin.Context) {
	var req struct {
		OldPath string `json:"old_path"`
		NewPath string `json:"new_path"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	if strings.TrimSpace(req.OldPath) == "" || strings.TrimSpace(req.NewPath) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "old_path and new_path required"})
		return
	}

	oldPath := filepath.Clean(req.OldPath)
	newPath := filepath.Clean(req.NewPath)
	if err := os.Rename(oldPath, newPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to rename: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "renamed", "path": newPath})
}

func (h *AdminFilesHandler) CopyFile(c *gin.Context) {
	var req struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	if strings.TrimSpace(req.Source) == "" || strings.TrimSpace(req.Destination) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source and destination required"})
		return
	}

	source := filepath.Clean(req.Source)
	dest := filepath.Clean(req.Destination)
	if err := copyPath(source, dest); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to copy: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "copied", "destination": dest})
}

func (h *AdminFilesHandler) MoveFile(c *gin.Context) {
	var req struct {
		Source      string `json:"source"`
		Destination string `json:"destination"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	if strings.TrimSpace(req.Source) == "" || strings.TrimSpace(req.Destination) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source and destination required"})
		return
	}

	source := filepath.Clean(req.Source)
	dest := filepath.Clean(req.Destination)
	if err := os.Rename(source, dest); err != nil {
		if err := copyPath(source, dest); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to move: " + err.Error()})
			return
		}
		_ = os.RemoveAll(source)
	}

	c.JSON(http.StatusOK, gin.H{"message": "moved", "destination": dest})
}

func (h *AdminFilesHandler) ChmodFile(c *gin.Context) {
	var req struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	if strings.TrimSpace(req.Path) == "" || strings.TrimSpace(req.Mode) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path and mode required"})
		return
	}

	modeValue, err := parseFileMode(req.Mode)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	fullPath := filepath.Clean(req.Path)
	if err := os.Chmod(fullPath, modeValue); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to change permission: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "permission updated", "path": fullPath})
}

func resolveOwnerGroup(info os.FileInfo) (string, string) {
	if runtime.GOOS == "windows" {
		return "-", "-"
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "-", "-"
	}

	uid := strconv.FormatUint(uint64(stat.Uid), 10)
	gid := strconv.FormatUint(uint64(stat.Gid), 10)
	owner := uid
	group := gid

	if u, err := user.LookupId(uid); err == nil {
		owner = u.Username
	}
	if g, err := user.LookupGroupId(gid); err == nil {
		group = g.Name
	}

	return owner, group
}

func formatPerm(mode os.FileMode) string {
	return fmt.Sprintf("%s%s%s%s%s%s%s%s%s",
		permBit(mode, 0400, "r"),
		permBit(mode, 0200, "w"),
		permBit(mode, 0100, "x"),
		permBit(mode, 0040, "r"),
		permBit(mode, 0020, "w"),
		permBit(mode, 0010, "x"),
		permBit(mode, 0004, "r"),
		permBit(mode, 0002, "w"),
		permBit(mode, 0001, "x"),
	)
}

func permBit(mode os.FileMode, bit os.FileMode, value string) string {
	if mode&bit != 0 {
		return value
	}
	return "-"
}

func parseFileMode(value string) (os.FileMode, error) {
	clean := strings.TrimSpace(value)
	clean = strings.TrimPrefix(clean, "0")
	if clean == "" {
		clean = "0"
	}
	if len(clean) > 4 {
		return 0, fmt.Errorf("invalid mode")
	}
	parsed, err := strconv.ParseUint(clean, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid mode")
	}
	return os.FileMode(parsed), nil
}

func copyPath(source, dest string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}
			target := filepath.Join(dest, rel)
			if info.IsDir() {
				return os.MkdirAll(target, info.Mode().Perm())
			}
			if !info.Mode().IsRegular() {
				return nil
			}
			return copyFile(path, target, info.Mode().Perm())
		})
	}

	return copyFile(source, dest, info.Mode().Perm())
}

func copyFile(source, dest string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	srcFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		_ = dstFile.Close()
		return err
	}
	if err := dstFile.Close(); err != nil {
		return err
	}

	return os.Chmod(dest, mode)
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
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	// We do NOT set cmd.Dir to dataVolumePath, let it start in default (usually home or working dir)

	if runtime.GOOS != "windows" {
		ptmx, err := pty.Start(cmd)
		if err != nil {
			conn.WriteMessage(websocket.TextMessage, []byte("Error starting PTY: "+err.Error()))
			return
		}
		defer func() {
			_ = ptmx.Close()
			_ = cmd.Wait()
		}()

		done := make(chan struct{})

		go func() {
			defer close(done)
			buf := make([]byte, 1024)
			for {
				n, err := ptmx.Read(buf)
				if n > 0 {
					if err := conn.WriteMessage(websocket.TextMessage, buf[:n]); err != nil {
						return
					}
				}
				if err != nil {
					return
				}
			}
		}()

		go func() {
			for {
				_, message, err := conn.ReadMessage()
				if err != nil {
					cancel()
					return
				}
				if _, err := ptmx.Write(message); err != nil {
					cancel()
					return
				}
			}
		}()

		<-done
		return
	}

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
		input := bytes.ReplaceAll(message, []byte("\r"), []byte("\r\n"))
		stdin.Write(input)
	}

	stdin.Close()
	cmd.Wait()
}

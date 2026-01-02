package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
)

type FileInfo struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	IsDir     bool      `json:"is_dir"`
	IsSymlink bool      `json:"is_symlink"`
	Size      int64     `json:"size"`
	Mode      string    `json:"mode"`
	ModTime   time.Time `json:"mod_time"`
}

func (c *Client) ListContainerFiles(ctx context.Context, containerID string, userID uint, path string) ([]FileInfo, error) {
	if c.api == nil {
		return nil, fmt.Errorf("docker client is not initialized")
	}

	_, err := c.GetContainerByID(ctx, containerID, userID)
	if err != nil {
		return nil, err
	}

	if path == "" {
		path = "/"
	}

	// Use Exec to run ls
	// ls -laN: -l (long), -a (all), -N (passed to some ls to not quote names, but standard ls doesn't always have it. Safe to omit)
	// We'll use "ls -la --time-style=long-iso" if available, but fallback gracefully?
	// Alpine (busybox) ls doesn't support --time-style.
	// Best common ground: "ls -la"
	cmd := []string{"ls", "-la", path}

	execConfig := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Cmd:          cmd,
	}

	execID, err := c.api.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		execConfig.Cmd = []string{"/bin/sh", "-c", "ls -la " + quoteShellPath(path)}
		execID, err = c.api.ContainerExecCreate(ctx, containerID, execConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create exec for ls: %w", err)
		}
	}

	resp, err := c.api.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to attach exec: %w", err)
	}
	defer resp.Close()

	output, err := io.ReadAll(resp.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read exec output: %w", err)
	}

	outputStr := string(output)

	return parseLsOutput(outputStr, path)
}

func quoteShellPath(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}

func stripAnsiCodes(s string) string {
	// Simple regex-like replacement for common ANSI sequences
	// \x1b[...m pattern
	result := s
	for {
		idx := strings.Index(result, "\x1b[")
		if idx == -1 {
			break
		}
		// Find the end of the sequence (usually 'm')
		endIdx := strings.IndexByte(result[idx:], 'm')
		if endIdx == -1 {
			break
		}
		result = result[:idx] + result[idx+endIdx+1:]
	}
	return result
}

// Quick parser for ls -la output
// drwxr-xr-x    1 root     root          4096 Jan  3 00:00 .
func parseLsOutput(output string, basePath string) ([]FileInfo, error) {
	lines := strings.Split(output, "\n")
	var files []FileInfo

	// Skip first line if it's "total X"
	startIndex := 0
	if len(lines) > 0 && strings.HasPrefix(lines[0], "total") {
		startIndex = 1
	}

	for _, line := range lines[startIndex:] {
		line = strings.TrimSpace(line)
		line = stripAnsiCodes(line) // Remove any ANSI color codes
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 8 {
			continue // Not a valid -l line
		}

		// Heuristic parsing
		// parts[0]: perms (drwxr-xr-x)
		// parts[1]: links
		// parts[2]: owner
		// parts[3]: group
		// parts[4]: size
		// parts[5-7]: date/time (Jan 3 00:00 or Jan 3 2024)
		// parts[8+]: name

		perms := parts[0]
		isDir := strings.HasPrefix(perms, "d")
		isSymlink := strings.HasPrefix(perms, "l")

		size, _ := strconv.ParseInt(parts[4], 10, 64)

		// Name is the rest
		name := strings.Join(parts[8:], " ")

		// Handle symlinks: "name -> target"
		if isSymlink {
			if idx := strings.Index(name, " -> "); idx != -1 {
				name = name[:idx]
			}
			isDir = true
		}

		if name == "." || name == ".." {
			continue
		}

		if name == "" {
			continue
		}

		files = append(files, FileInfo{
			Name:      name,
			Path:      filepath.Join(basePath, name),
			IsDir:     isDir,
			IsSymlink: isSymlink,
			Size:      size,
			Mode:      perms,
			ModTime:   time.Now(),
		})
	}

	return files, nil
}

func (c *Client) ReadContainerFile(ctx context.Context, containerID string, userID uint, path string) ([]byte, error) {
	if c.api == nil {
		return nil, fmt.Errorf("docker client is not initialized")
	}

	_, err := c.GetContainerByID(ctx, containerID, userID)
	if err != nil {
		return nil, err
	}

	reader, _, err := c.api.CopyFromContainer(ctx, containerID, path)
	if err != nil {
		return nil, fmt.Errorf("failed to copy from container: %w", err)
	}
	defer reader.Close()

	tr := tar.NewReader(reader)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		if header.Typeflag == tar.TypeReg {
			return io.ReadAll(tr)
		}
	}

	return nil, fmt.Errorf("file not found in archive")
}

func (c *Client) WriteContainerFile(ctx context.Context, containerID string, userID uint, path string, content []byte) error {
	if c.api == nil {
		return fmt.Errorf("docker client is not initialized")
	}

	_, err := c.GetContainerByID(ctx, containerID, userID)
	if err != nil {
		return err
	}

	// Create a tar archive
	buf := new(bytes.Buffer)
	tw := tar.NewWriter(buf)

	header := &tar.Header{
		Name:    filepath.Base(path),
		Size:    int64(len(content)),
		Mode:    0644,
		ModTime: time.Now(),
	}

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	if _, err := tw.Write(content); err != nil {
		return err
	}

	if err := tw.Close(); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	return c.api.CopyToContainer(ctx, containerID, dir, buf, container.CopyToContainerOptions{})
}

func (c *Client) CreateContainerDir(ctx context.Context, containerID string, userID uint, path string) error {
	if c.api == nil {
		return fmt.Errorf("docker client is not initialized")
	}

	_, err := c.GetContainerByID(ctx, containerID, userID)
	if err != nil {
		return err
	}

	cmd := []string{"mkdir", "-p", path}

	execConfig := container.ExecOptions{
		AttachStderr: true,
		Cmd:          cmd,
	}

	execID, err := c.api.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return err
	}

	return c.api.ContainerExecStart(ctx, execID.ID, container.ExecStartOptions{})
}

func (c *Client) DeleteContainerFile(ctx context.Context, containerID string, userID uint, path string) error {
	if c.api == nil {
		return fmt.Errorf("docker client is not initialized")
	}

	_, err := c.GetContainerByID(ctx, containerID, userID)
	if err != nil {
		return err
	}

	cmd := []string{"rm", "-rf", path}

	execConfig := container.ExecOptions{
		AttachStderr: true,
		Cmd:          cmd,
	}

	execID, err := c.api.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return err
	}

	return c.api.ContainerExecStart(ctx, execID.ID, container.ExecStartOptions{})
}

func (c *Client) RenameContainerFile(ctx context.Context, containerID string, userID uint, oldPath, newPath string) error {
	if c.api == nil {
		return fmt.Errorf("docker client is not initialized")
	}

	_, err := c.GetContainerByID(ctx, containerID, userID)
	if err != nil {
		return err
	}

	cmd := []string{"mv", oldPath, newPath}

	execConfig := container.ExecOptions{
		AttachStderr: true,
		Cmd:          cmd,
	}

	execID, err := c.api.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return err
	}

	return c.api.ContainerExecStart(ctx, execID.ID, container.ExecStartOptions{})
}

func (c *Client) CopyContainerFile(ctx context.Context, containerID string, userID uint, source, destination string) error {
	if c.api == nil {
		return fmt.Errorf("docker client is not initialized")
	}

	_, err := c.GetContainerByID(ctx, containerID, userID)
	if err != nil {
		return err
	}

	cmd := []string{"cp", "-r", source, destination}

	execConfig := container.ExecOptions{
		AttachStderr: true,
		Cmd:          cmd,
	}

	execID, err := c.api.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return err
	}

	return c.api.ContainerExecStart(ctx, execID.ID, container.ExecStartOptions{})
}

func (c *Client) MoveContainerFile(ctx context.Context, containerID string, userID uint, source, destination string) error {
	return c.RenameContainerFile(ctx, containerID, userID, source, destination)
}

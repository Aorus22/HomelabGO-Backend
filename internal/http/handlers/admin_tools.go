package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/showwin/speedtest-go/speedtest"
)

type AdminToolsHandler struct{}

func NewAdminToolsHandler() *AdminToolsHandler {
	return &AdminToolsHandler{}
}

type speedtestResult struct {
	Download float64 `json:"download"` // Mbps
	Upload   float64 `json:"upload"`   // Mbps
	Ping     float64 `json:"ping"`     // ms
	Server   string  `json:"server"`
	Error    string  `json:"error,omitempty"`
}

// RunSpeedtest performs a speed test using pure Go library
func (h *AdminToolsHandler) RunSpeedtest(c *gin.Context) {
	// Create speedtest client with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()

	// Get user config (for finding closest server)
	user, err := speedtest.FetchUserInfo()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch user info: " + err.Error(),
		})
		return
	}

	// Get server list
	serverList, err := speedtest.FetchServers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch servers: " + err.Error(),
		})
		return
	}

	// Find closest servers
	targets, err := serverList.FindServer([]int{})
	if err != nil || len(targets) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "No servers available",
		})
		return
	}

	// Use the first (closest) server
	server := targets[0]

	// Run ping test
	if err := server.PingTestContext(ctx, nil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Ping test failed: " + err.Error(),
		})
		return
	}

	// Run download test
	if err := server.DownloadTestContext(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Download test failed: " + err.Error(),
		})
		return
	}

	// Run upload test
	if err := server.UploadTestContext(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Upload test failed: " + err.Error(),
		})
		return
	}

	// Build result
	result := speedtestResult{
		Download: float64(server.DLSpeed),
		Upload:   float64(server.ULSpeed),
		Ping:     float64(server.Latency.Milliseconds()),
		Server:   server.Name + " (" + server.Sponsor + ")",
	}

	// Log for debugging
	_ = user // suppress unused warning

	c.JSON(http.StatusOK, result)
}

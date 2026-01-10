package handlers

import (
	"net/http"

	"homelabgo/internal/system"

	"github.com/gin-gonic/gin"
)

type AdminSystemHandler struct{}

func NewAdminSystemHandler() *AdminSystemHandler {
	return &AdminSystemHandler{}
}

func (h *AdminSystemHandler) GetCron(c *gin.Context) {
	jobs, err := system.GetCrontab()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, jobs)
}

func (h *AdminSystemHandler) SaveCron(c *gin.Context) {
	var jobs []system.CronJob
	if err := c.ShouldBindJSON(&jobs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid content"})
		return
	}

	if err := system.SaveCronJobs(jobs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *AdminSystemHandler) ListServices(c *gin.Context) {
	services, err := system.ListServices()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, services)
}

func (h *AdminSystemHandler) ServiceAction(c *gin.Context) {
	id := c.Param("id") // Service name
	var req struct {
		Action string `json:"action"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "action required"})
		return
	}

	if err := system.ServiceAction(id, req.Action); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *AdminSystemHandler) CreateService(c *gin.Context) {
	var config system.ServiceConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid content"})
		return
	}

	if err := system.CreateService(config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *AdminSystemHandler) DeleteService(c *gin.Context) {
	name := c.Param("id")
	if err := system.DeleteService(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *AdminSystemHandler) GetServiceLogs(c *gin.Context) {
	name := c.Param("id")
	logs, err := system.GetServiceLogs(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

func (h *AdminSystemHandler) ListPorts(c *gin.Context) {
	ports, err := system.GetOpenPorts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ports)
}

func (h *AdminSystemHandler) ListNetworks(c *gin.Context) {
	ifaces, err := system.GetNetworkInterfaces()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ifaces)
}

func (h *AdminSystemHandler) ListProcesses(c *gin.Context) {
	procs, err := system.GetProcesses()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, procs)
}

func (h *AdminSystemHandler) KillProcess(c *gin.Context) {
	pid := c.Param("pid")
	if pid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "PID required"})
		return
	}

	if err := system.KillProcess(pid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "killed"})
}

func (h *AdminSystemHandler) GetFirewall(c *gin.Context) {
	status, err := system.GetFirewallStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, status)
}

func (h *AdminSystemHandler) ToggleFirewall(c *gin.Context) {
	var req struct {
		Enable bool `json:"enable"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := system.ToggleFirewall(req.Enable); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *AdminSystemHandler) AddFirewallRule(c *gin.Context) {
	var req struct {
		Port   string `json:"port"`
		Proto  string `json:"proto"`
		Action string `json:"action"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := system.AddFirewallRule(req.Port, req.Proto, req.Action); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *AdminSystemHandler) DeleteFirewallRule(c *gin.Context) {
	id := c.Param("id")
	if err := system.DeleteFirewallRule(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *AdminSystemHandler) ListRcloneRemotes(c *gin.Context) {
	remotes, err := system.ListRcloneRemotes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, remotes)
}

func (h *AdminSystemHandler) CreateRcloneRemote(c *gin.Context) {
	var req struct {
		Name     string            `json:"name"`
		Provider string            `json:"provider"`
		Config   map[string]string `json:"config"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := system.CreateRcloneRemote(req.Name, req.Provider, req.Config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "created"})
}

func (h *AdminSystemHandler) DeleteRcloneRemote(c *gin.Context) {
	name := c.Param("name")
	if err := system.DeleteRcloneRemote(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *AdminSystemHandler) MountRcloneRemote(c *gin.Context) {
	var req struct {
		Remote    string `json:"remote"`
		LocalPath string `json:"path"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := system.MountRcloneRemote(req.Remote, req.LocalPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "mounted"})
}

func (h *AdminSystemHandler) SyncRclone(c *gin.Context) {
	var req struct {
		Source string `json:"source"`
		Dest   string `json:"dest"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := system.SyncRclone(req.Source, req.Dest); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "sync_started"})
}

func (h *AdminSystemHandler) GetRcloneStatus(c *gin.Context) {
	installed := system.IsRcloneInstalled()
	c.JSON(http.StatusOK, gin.H{"installed": installed})
}

func (h *AdminSystemHandler) InstallRclone(c *gin.Context) {
	if err := system.InstallRclone(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "installed"})
}

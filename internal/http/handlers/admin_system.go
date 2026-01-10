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

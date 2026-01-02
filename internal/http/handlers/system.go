package handlers

import (
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
)

func GetSystemStats(c *gin.Context) {
	v, _ := mem.VirtualMemory()
	c_percent, _ := cpu.Percent(0, false)
	d, _ := disk.Usage("/")
	h, _ := host.Info()

	cpuVal := 0.0
	if len(c_percent) > 0 {
		cpuVal = c_percent[0]
	}

	c.JSON(http.StatusOK, gin.H{
		"cpu_percent":    cpuVal,
		"memory_percent": v.UsedPercent,
		"disk_percent":   d.UsedPercent,
		"host_info": gin.H{
			"hostname":       h.Hostname,
			"uptime":         h.Uptime,
			"platform":       h.Platform,
			"kernel_version": h.KernelVersion,
			"go_version":     runtime.Version(),
			"time":           time.Now(),
		},
	})
}

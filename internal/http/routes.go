package http

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"homelabgo/internal/config"
	"homelabgo/internal/docker"
	"homelabgo/internal/http/handlers"
	"homelabgo/internal/http/middleware"
)

type Dependencies struct {
	DB     *gorm.DB
	Config config.Config
	Docker *docker.Client
}

func RegisterRoutes(router *gin.Engine, deps Dependencies) {
	authHandler := handlers.NewAuthHandler(deps.DB, deps.Config.JWTSecret, deps.Docker)
	volumeHandler := handlers.NewVolumeHandler(deps.DB, deps.Docker)
	deploymentHandler := handlers.NewDeploymentHandler(deps.DB, deps.Docker)
	containerHandler := handlers.NewContainerHandler(deps.DB, deps.Docker)
	containerFileHandler := handlers.NewContainerFileHandler(deps.Docker)
	fileHandler := handlers.NewFileHandler(deps.DB, deps.Config.DataVolumePath)
	cloudflareHandler := handlers.NewCloudflareHandler(deps.DB, deps.Docker)
	envFileHandler := handlers.NewEnvFileHandler(deps.DB)
	wsHandler := handlers.NewWebSocketHandler(deps.Docker, deps.Config.JWTSecret)

	router.GET("/health", handlers.Health)

	authGroup := router.Group("/auth")
	authGroup.POST("/register", authHandler.Register)
	authGroup.POST("/login", authHandler.Login)
	authGroup.GET("/me", middleware.JWTAuth(deps.Config.JWTSecret), authHandler.Me)

	protected := router.Group("")
	protected.Use(middleware.JWTAuth(deps.Config.JWTSecret))
	{
		protected.GET("/system/stats", handlers.GetSystemStats)

		volumes := protected.Group("/volumes")
		{
			volumes.GET("", volumeHandler.List)
			volumes.POST("", volumeHandler.Create)
			volumes.DELETE("/:id", volumeHandler.Delete)
			volumes.GET("/:id/download", volumeHandler.Download)
			volumes.POST("/upload", volumeHandler.Upload)
		}

		deployments := protected.Group("/deployments")
		{
			deployments.GET("", deploymentHandler.List)
			deployments.POST("", deploymentHandler.Create)
			deployments.GET("/:id", deploymentHandler.Get)
			deployments.PUT("/:id", deploymentHandler.Update)
			deployments.DELETE("/:id", deploymentHandler.Delete)
			deployments.POST("/:id/validate", deploymentHandler.Validate)
			deployments.POST("/:id/deploy", deploymentHandler.Deploy)
			deployments.POST("/:id/stop", deploymentHandler.Stop)
			deployments.POST("/:id/start", deploymentHandler.Start)
		}

		containers := protected.Group("/containers")
		{
			containers.GET("", containerHandler.List)
			containers.GET("/:id", containerHandler.Get)
			containers.POST("/:id/start", containerHandler.Start)
			containers.POST("/:id/stop", containerHandler.Stop)
			containers.POST("/:id/restart", containerHandler.Restart)
			containers.POST("/:id/recreate", containerHandler.Recreate)
			containers.POST("/:id/pull", containerHandler.Pull)
			containers.GET("/:id/logs", containerHandler.Logs)
			containers.GET("/:id/stats", containerHandler.Stats)
			containers.GET("/:id/mounts", containerHandler.Mounts)

			containerFiles := containers.Group("/:id/files")
			{
				containerFiles.GET("", containerFileHandler.ListFiles)
				containerFiles.GET("/content", containerFileHandler.GetFileContent)
				containerFiles.GET("/download", containerFileHandler.DownloadFile)
				containerFiles.POST("/upload", containerFileHandler.UploadFile)
				containerFiles.PUT("", containerFileHandler.SaveFileContent)
				containerFiles.POST("/mkdir", containerFileHandler.CreateDirectory)
				containerFiles.DELETE("", containerFileHandler.DeleteFile)
				containerFiles.POST("/rename", containerFileHandler.RenameFile)
				containerFiles.POST("/copy", containerFileHandler.CopyFile)
				containerFiles.POST("/move", containerFileHandler.MoveFile)
			}
		}

		files := protected.Group("/files")
		{
			files.GET("", fileHandler.List)
			files.GET("/*path", fileHandler.GetFile)
			files.PUT("/*path", fileHandler.SaveFile)
			files.DELETE("/*path", fileHandler.DeleteFile)
			files.POST("/upload", fileHandler.Upload)
		}

		cloudflare := protected.Group("/cloudflare")
		{
			cloudflare.GET("", cloudflareHandler.GetConfig)
			cloudflare.PUT("", cloudflareHandler.UpdateConfig)
			cloudflare.GET("/status", cloudflareHandler.GetStatus)
			cloudflare.GET("/logs", cloudflareHandler.GetLogs)
		}

		envFiles := protected.Group("/envfiles")
		{
			envFiles.GET("", envFileHandler.List)
			envFiles.POST("", envFileHandler.Create)
			envFiles.GET("/:id", envFileHandler.Get)
			envFiles.PUT("/:id", envFileHandler.Update)
			envFiles.DELETE("/:id", envFileHandler.Delete)
		}
	}

	ws := router.Group("/ws")
	{
		ws.GET("/logs/:container_id", wsHandler.StreamLogs)
		ws.GET("/exec/:container_id", wsHandler.ExecTerminal)
	}

	// Admin routes
	adminHandler := handlers.NewAdminHandler(deps.DB, deps.Docker)
	adminToolsHandler := handlers.NewAdminToolsHandler()
	adminCloudflareHandler := handlers.NewAdminCloudflareHandler(deps.DB, deps.Docker)

	admin := router.Group("/admin")
	admin.Use(middleware.JWTAuth(deps.Config.JWTSecret), middleware.AdminOnly())
	{
		// Users management
		admin.GET("/users", adminHandler.ListUsers)
		admin.GET("/users/:id", adminHandler.GetUser)
		admin.GET("/users/:id/deployments", adminHandler.GetUserDeployments)
		admin.GET("/users/:id/deployments/:did/containers", adminHandler.GetDeploymentContainers)
		admin.GET("/users/:id/volumes", adminHandler.GetUserVolumes)
		admin.GET("/users/:id/envfiles", adminHandler.GetUserEnvFiles)

		// All containers
		admin.GET("/containers", adminHandler.ListAllContainers)

		// Docker Management
		adminDockerHandler := handlers.NewAdminDockerHandler(deps.DB, deps.Docker)
		dockerGroup := admin.Group("/docker")
		{
			dockerGroup.GET("/containers", adminDockerHandler.ListContainers)
			dockerGroup.GET("/images", adminDockerHandler.ListImages)
			dockerGroup.GET("/networks", adminDockerHandler.ListNetworks)
			dockerGroup.GET("/volumes", adminDockerHandler.ListVolumes)
		}

		// Tools
		admin.POST("/tools/speedtest", adminToolsHandler.RunSpeedtest)

		// Admin Files
		adminFilesHandler := handlers.NewAdminFilesHandler(deps.Config.DataVolumePath)
		admin.GET("/files", adminFilesHandler.ListFiles)
		admin.GET("/files/content", adminFilesHandler.GetFile)
		admin.POST("/files/content", adminFilesHandler.SaveFile)

		// Terminal WS (needs specific handling for WS upgrade in protected route)
		// Usually WS requests don't send Authorization header, so we pass token in query
		// But here we are inside a middleware that expects header.
		// Ideally we register this outside or handle auth in handler.
		// For now, let's keep it here but assume client sends header (which RN can do for WS)
		admin.GET("/tools/terminal", adminFilesHandler.HostTerminal)

		// Cloudflare instances
		admin.GET("/cloudflare", adminCloudflareHandler.List)
		admin.POST("/cloudflare", adminCloudflareHandler.Create)
		admin.POST("/cloudflare/:id/start", adminCloudflareHandler.Start)
		admin.POST("/cloudflare/:id/stop", adminCloudflareHandler.Stop)
		admin.DELETE("/cloudflare/:id", adminCloudflareHandler.Delete)

		// System Management (Cron & Services)
		adminSystemHandler := handlers.NewAdminSystemHandler()
		systemGroup := admin.Group("/system")
		{
			systemGroup.GET("/cron", adminSystemHandler.GetCron)
			systemGroup.POST("/cron", adminSystemHandler.SaveCron)
			// System Services
			systemRoutes := systemGroup.Group("/services")
			{
				systemRoutes.GET("", adminSystemHandler.ListServices)
				systemRoutes.POST("", adminSystemHandler.CreateService)
				systemRoutes.POST("/:id/action", adminSystemHandler.ServiceAction)
				systemRoutes.GET("/:id/logs", adminSystemHandler.GetServiceLogs)
				systemRoutes.DELETE("/:id", adminSystemHandler.DeleteService)
			}

			// Ports & Networks
			systemGroup.GET("/ports", adminSystemHandler.ListPorts)
			systemGroup.GET("/networks", adminSystemHandler.ListNetworks)

			// Process Manager
			systemGroup.GET("/processes", adminSystemHandler.ListProcesses)
			systemGroup.DELETE("/processes/:pid", adminSystemHandler.KillProcess)

			// Firewall
			systemGroup.GET("/firewall", adminSystemHandler.GetFirewall)
			systemGroup.POST("/firewall/toggle", adminSystemHandler.ToggleFirewall)
			systemGroup.POST("/firewall/rules", adminSystemHandler.AddFirewallRule)
			systemGroup.DELETE("/firewall/rules/:id", adminSystemHandler.DeleteFirewallRule)
		}
	}
}

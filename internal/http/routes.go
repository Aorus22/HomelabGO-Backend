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
	wsHandler := handlers.NewWebSocketHandler(deps.Docker, deps.Config.JWTSecret)

	router.GET("/health", handlers.Health)

	authGroup := router.Group("/auth")
	authGroup.POST("/register", authHandler.Register)
	authGroup.POST("/login", authHandler.Login)

	protected := router.Group("")
	protected.Use(middleware.JWTAuth(deps.Config.JWTSecret))
	{
		protected.GET("/system/stats", handlers.GetSystemStats)

		volumes := protected.Group("/volumes")
		{
			volumes.GET("", volumeHandler.List)
			volumes.POST("", volumeHandler.Create)
			volumes.DELETE("/:id", volumeHandler.Delete)
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

			containerFiles := containers.Group("/:id/files")
			{
				containerFiles.GET("", containerFileHandler.ListFiles)
				containerFiles.GET("/content", containerFileHandler.GetFileContent)
				containerFiles.GET("/download", containerFileHandler.DownloadFile)
				containerFiles.POST("/upload", containerFileHandler.UploadFile)
				containerFiles.PUT("", containerFileHandler.SaveFileContent)
				containerFiles.POST("/mkdir", containerFileHandler.CreateDirectory)
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
	}

	ws := router.Group("/ws")
	{
		ws.GET("/logs/:container_id", wsHandler.StreamLogs)
		ws.GET("/exec/:container_id", wsHandler.ExecTerminal)
	}
}

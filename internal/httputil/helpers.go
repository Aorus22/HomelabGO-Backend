package httputil

import (
	"github.com/gin-gonic/gin"
)

func GetUserID(c *gin.Context) uint {
	userID, exists := c.Get("userID")
	if !exists {
		return 0
	}
	return userID.(uint)
}

func GetUserRole(c *gin.Context) string {
	role, exists := c.Get("role")
	if !exists {
		return ""
	}
	return role.(string)
}

func IsAdmin(c *gin.Context) bool {
	return GetUserRole(c) == "admin"
}

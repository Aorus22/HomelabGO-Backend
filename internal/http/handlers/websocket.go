package handlers

import (
	"bufio"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"homelabgo/internal/auth"
	"homelabgo/internal/docker"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type WebSocketHandler struct {
	docker    *docker.Client
	jwtSecret string
}

func NewWebSocketHandler(dockerClient *docker.Client, jwtSecret string) *WebSocketHandler {
	return &WebSocketHandler{
		docker:    dockerClient,
		jwtSecret: jwtSecret,
	}
}

func (h *WebSocketHandler) authenticateWebSocket(c *gin.Context) (uint, string, error) {
	token := c.Query("token")
	if token == "" {
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				token = parts[1]
			}
		}
	}

	if token == "" {
		return 0, "", http.ErrNoCookie
	}

	claims, err := auth.ParseToken(token, h.jwtSecret)
	if err != nil {
		return 0, "", err
	}

	return claims.UserID, claims.Role, nil
}

func (h *WebSocketHandler) StreamLogs(c *gin.Context) {
	userID, role, err := h.authenticateWebSocket(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if role == "admin" {
		userID = 0
	}

	containerID := c.Param("container_id")
	if containerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "container_id required"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	logReader, err := h.docker.StreamContainerLogs(c.Request.Context(), containerID, userID)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Error: "+err.Error()))
		return
	}
	defer logReader.Close()

	scanner := bufio.NewScanner(logReader)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 8 {
			line = line[8:]
		}
		if err := conn.WriteMessage(websocket.TextMessage, []byte(line)); err != nil {
			log.Printf("WebSocket write error: %v", err)
			return
		}
	}
}

func (h *WebSocketHandler) ExecTerminal(c *gin.Context) {
	userID, role, err := h.authenticateWebSocket(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if role == "admin" {
		userID = 0
	}

	containerID := c.Param("container_id")
	if containerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "container_id required"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	shell := c.Query("shell")

	execResp, _, err := h.docker.ExecContainer(c.Request.Context(), containerID, userID, shell)
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte("Error: "+err.Error()))
		return
	}
	defer execResp.Close()

	done := make(chan struct{})

	go func() {
		defer close(done)
		buf := make([]byte, 1024)
		for {
			n, err := execResp.Reader.Read(buf)
			if err != nil {
				if err != io.EOF {
					log.Printf("Docker read error: %v", err)
				}
				return
			}
			if n > 0 {
				log.Printf("Read %d bytes from Docker", n)
				if err := conn.WriteMessage(websocket.TextMessage, buf[:n]); err != nil {
					log.Printf("WebSocket write error: %v", err)
					return
				}
			}
		}
	}()

	go func() {
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					log.Printf("WebSocket read error: %v", err)
				}
				return
			}
			log.Printf("Received %d bytes from WS: %q", len(message), string(message))
			if _, err := execResp.Conn.Write(message); err != nil {
				log.Printf("Docker write error: %v", err)
				return
			}
		}
	}()

	<-done
}

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type Peer struct {
	Username string `json:"username"`
	IP       string `json:"ip"`
	Port     string `json:"port"`
}

var (
	peers      = make(map[string]Peer)
	clients    = make(map[*websocket.Conn]bool)
	broadcast  = make(chan Peer)
	mu         sync.Mutex
	wsUpgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

func registerPeer(c *gin.Context) {
	var peer Peer
	if err := c.BindJSON(&peer); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
		return
	}

	mu.Lock()
	peers[peer.Username] = peer
	mu.Unlock()

	broadcast <- peer
	c.JSON(http.StatusOK, gin.H{"status": "registered"})
}

func getPeers(c *gin.Context) {
	mu.Lock()
	defer mu.Unlock()

	peerList := make([]Peer, 0, len(peers))
	for _, peer := range peers {
		peerList = append(peerList, peer)
	}
	c.JSON(http.StatusOK, peerList)
}

func handleWS(c *gin.Context) {
	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		fmt.Println("WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()
	clients[conn] = true

	for {
		// Keep connection open
		_, _, err := conn.ReadMessage()
		if err != nil {
			delete(clients, conn)
			break
		}
	}
}

func startBroadcast() {
	for {
		peer := <-broadcast
		data, _ := json.Marshal(peer)
		for client := range clients {
			err := client.WriteMessage(websocket.TextMessage, data)
			if err != nil {
				client.Close()
				delete(clients, client)
			}
		}
	}
}

func main() {
	r := gin.Default()
	r.POST("/register", registerPeer)
	r.GET("/peers", getPeers)
	r.GET("/ws", func(c *gin.Context) {
		handleWS(c)
	})

	go startBroadcast()

	r.Run(":8080")
}
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"p2p-bootstrap-server/p2p"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var (
	peers      = make(map[string]p2p.Peer)
	clients    = make(map[*websocket.Conn]bool)
	broadcast  = make(chan p2p.Peer)
	mu         sync.RWMutex
	wsUpgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

func registerPeer(c *gin.Context) {
	var peer p2p.Peer
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
	mu.RLock()
	defer mu.RUnlock()

	peerList := make([]p2p.Peer, 0, len(peers))
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
		data, err := json.Marshal(peer)
		if err != nil {
			log.Printf("Error marshalling peer; %v\n", err)
			continue
		}

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

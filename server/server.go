package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type Peer struct {
	Username string    `json:"username"`
	IP       string    `json:"ip"`
	Port     string    `json:"port"`
	LastSeen time.Time `json:"lastSeen"`
}

var (
	peers     = make(map[string]Peer)
	mu        sync.Mutex
	dbFile    = "peers.json"
	pruneAfter = 5 * time.Minute // Remove peers inactive for 5+ minutes
	maxPeersToSend = 10          // Limit number of peers returned
)

// Load peers from disk
func loadPeers() {
	file, err := os.ReadFile(dbFile)
	if err != nil {
		fmt.Println("No existing peer list.")
		return
	}
	var loadedPeers []Peer
	if err := json.Unmarshal(file, &loadedPeers); err != nil {
		fmt.Println("Error decoding peer file:", err)
		return
	}
	mu.Lock()
	defer mu.Unlock()
	for _, peer := range loadedPeers {
		peers[peer.Username] = peer
	}
	fmt.Println("Loaded peers from disk:", len(peers))
}

// Save peers to disk
func savePeers() {
	mu.Lock()
	defer mu.Unlock()

	peerList := make([]Peer, 0, len(peers))
	for _, peer := range peers {
		peerList = append(peerList, peer)
	}
	data, err := json.MarshalIndent(peerList, "", "  ")
	if err != nil {
		fmt.Println("Error marshaling peers:", err)
		return
	}
	if err := os.WriteFile(dbFile, data, 0644); err != nil {
		fmt.Println("Error writing peer file:", err)
	}
}

// Register new or existing peer
func registerPeer(c *gin.Context) {
	var peer Peer
	if err := c.BindJSON(&peer); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
		return
	}
	peer.LastSeen = time.Now()

	mu.Lock()
	peers[peer.Username] = peer
	mu.Unlock()

	savePeers()
	c.JSON(http.StatusOK, gin.H{"status": "registered"})
}

// Update LastSeen time via /ping
func pingPeer(c *gin.Context) {
	username := c.Query("username")
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Username required"})
		return
	}
	mu.Lock()
	defer mu.Unlock()
	peer, ok := peers[username]
	if ok {
		peer.LastSeen = time.Now()
		peers[username] = peer
		savePeers()
		c.JSON(http.StatusOK, gin.H{"status": "pong"})
	} else {
		c.JSON(http.StatusNotFound, gin.H{"error": "Peer not found"})
	}
}

// Return a random sample of peers
func getPeers(c *gin.Context) {
	mu.Lock()
	defer mu.Unlock()

	peerList := make([]Peer, 0, len(peers))
	for _, peer := range peers {
		peerList = append(peerList, peer)
	}

	// Shuffle and limit
	rand.Shuffle(len(peerList), func(i, j int) {
		peerList[i], peerList[j] = peerList[j], peerList[i]
	})
	if len(peerList) > maxPeersToSend {
		peerList = peerList[:maxPeersToSend]
	}
	c.JSON(http.StatusOK, peerList)
}

// Periodically prune stale peers
func startPeerPruner() {
	ticker := time.NewTicker(1 * time.Minute)
	for range ticker.C {
		mu.Lock()
		for k, peer := range peers {
			if time.Since(peer.LastSeen) > pruneAfter {
				fmt.Println("Pruning stale peer:", peer.Username)
				delete(peers, k)
			}
		}
		mu.Unlock()
		savePeers()
	}
}

func main() {
	loadPeers()

	go startPeerPruner()

	r := gin.Default()
	r.POST("/register", registerPeer)
	r.GET("/peers", getPeers)
	r.GET("/ping", pingPeer)

	fmt.Println("Bootnode running on port 8080...")
	r.Run(":8080")
}

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
)

type Peer struct {
	Username string `json:"username"`
	IP       string `json:"ip"`
	Port     string `json:"port"`
}

var (
	peerList = make(map[string]Peer)
	self     Peer
)

// Register this peer with the bootstrap server
func registerWithServer(serverURL, username, port string) error {
	localIP := getLocalIP()
	self = Peer{
		Username: username,
		IP:       localIP,
		Port:     port,
	}
	data, _ := json.Marshal(self)
	_, err := http.Post(serverURL+"/register", "application/json", bytes.NewBuffer(data))
	return err
}

// Listen for new peer joins via WebSocket
func listenToWS(serverURL string) {
	conn, _, err := websocket.DefaultDialer.Dial("ws://"+serverURL+"/ws", nil)
	if err != nil {
		log.Fatal("WebSocket dial error:", err)
	}
	defer conn.Close()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Println("WebSocket read error:", err)
			return
		}
		var newPeer Peer
		json.Unmarshal(message, &newPeer)

		if newPeer.Username == self.Username {
			continue // ignore self
		}
		peerList[newPeer.Username] = newPeer
		fmt.Printf("[DISCOVERY] New peer joined: %s (%s:%s)\n", newPeer.Username, newPeer.IP, newPeer.Port)
	}
}

// Fetch peers from bootstrap on startup
func getInitialPeers(serverURL string) {
	resp, err := http.Get(serverURL + "/peers")
	if err != nil {
		log.Println("Error fetching peers:", err)
		return
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)

	var peers []Peer
	json.Unmarshal(body, &peers)
	for _, p := range peers {
		if p.Username != self.Username {
			peerList[p.Username] = p
			fmt.Printf("[BOOTSTRAP] Peer: %s (%s:%s)\n", p.Username, p.IP, p.Port)
		}
	}
}

// Start listening for UDP P2P messages
func startP2PListener(port string) {
	addr := ":" + port
	udpAddr, _ := net.ResolveUDPAddr("udp", addr)
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatalf("Failed to bind to port %s: %v\n", port, err)
	}
	defer conn.Close()
	fmt.Println("[LISTENING] Waiting for P2P messages on", addr)

	buf := make([]byte, 1024)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Println("Read error:", err)
			continue
		}
		fmt.Printf("[MESSAGE] Received from %s: %s\n", remoteAddr, string(buf[:n]))
	}
}

// Send a message to a specific peer
func sendMessageToPeer(username, message string) {
	peer, exists := peerList[username]
	if !exists {
		fmt.Println("[ERROR] Peer not found:", username)
		return
	}
	addr := net.JoinHostPort(peer.IP, peer.Port)
	udpAddr, _ := net.ResolveUDPAddr("udp", addr)
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		log.Printf("[ERROR] Failed to dial %s: %v\n", username, err)
		return
	}
	defer conn.Close()

	conn.Write([]byte(message))
	fmt.Printf("[SENT] ➡️  Message to %s (%s:%s): %s\n", peer.Username, peer.IP, peer.Port, message)
}

// Send periodic pings to all known peers
func startPeerInteractionLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		for username := range peerList {
			msg := fmt.Sprintf("PING from %s", self.Username)
			sendMessageToPeer(username, msg)
		}
	}
}

// Get local machine IP address
func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

// Entry point
func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: go run main.go <username> <port> <bootstrap-host:port>")
		return
	}
	username := os.Args[1]
	port := os.Args[2]
	serverAddr := os.Args[3]

	err := registerWithServer("http://"+serverAddr, username, port)
	if err != nil {
		log.Fatalln("Failed to register with bootstrap server:", err)
	}

	getInitialPeers("http://" + serverAddr)

	go listenToWS(serverAddr)
	go startP2PListener(port)
	go startPeerInteractionLoop()

	// Graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	fmt.Println("\n[EXIT] Shutting down.")
}

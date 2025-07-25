package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"p2p-bootstrap-server/p2p"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

var (
	peerList      = make(map[string]p2p.Peer)
	self          p2p.Peer
	username      string
	serverAddress string
)

func init() {
	flag.StringVar(&username, "username", "", "Username to join the server as")
	flag.StringVar(&serverAddress, "server", "", "Address of bootstrap server. Format [host:port]")
	flag.Parse()

	if strings.TrimSpace(username) == "" || strings.TrimSpace(serverAddress) == "" {
		flag.Usage()
		os.Exit(1)
	}

	err := p2p.ValidateIPAddress(serverAddress)
	if err != nil {
		log.Fatalln(err)
	}

	self = p2p.Peer{
		Username: username,
		IP:       p2p.GetLocalIP(),
		Port:     p2p.GenerateRandomPort(),
	}
}

// Listen for new peer joins via WebSocket
func listenToWS() {
	endpoint := fmt.Sprintf("ws://%v/ws", serverAddress)
	log.Printf("Client connecting to %v...\n", endpoint)

	conn, _, err := websocket.DefaultDialer.Dial(endpoint, nil)
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

		var newPeer p2p.Peer
		err = json.Unmarshal(message, &newPeer)
		if err != nil {
			log.Printf("Error unmarshalling received message; %v\n", err)
			continue
		}

		if newPeer.Username == self.Username {
			continue // ignore self
		}
		peerList[newPeer.Username] = newPeer
		fmt.Printf("[DISCOVERY] New peer joined: %s (%s:%s)\n", newPeer.Username, newPeer.IP, newPeer.Port)
	}
}

// Fetch peers from bootstrap on startup
func getInitialPeers() {
	endpoint := fmt.Sprintf("http://%v/peers", serverAddress)
	log.Printf("Fetching initial peers from %v...\n", endpoint)

	resp, err := http.Get(endpoint)
	if err != nil {
		log.Println("Error fetching peers:", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response body; %v\n", err)
		return
	}

	var peers []p2p.Peer
	json.Unmarshal(body, &peers)
	for _, p := range peers {
		if p.Username != self.Username {
			peerList[p.Username] = p
			fmt.Printf("[BOOTSTRAP] Peer: %s (%s:%s)\n", p.Username, p.IP, p.Port)
		}
	}
}

// Start listening for UDP P2P messages
func startP2PListener() {
	addr := fmt.Sprintf(":%v", self.Port)
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		log.Fatalf("[ERROR] Resolving UDP address; %v\n", err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatalf("[ERROR] Binding to port %s: %v\n", self.Port, err)
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
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		log.Fatalf("[ERROR] Resolving UDP address; %v", err)
	}

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
func pingKnownPeers() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		for username := range peerList {
			msg := fmt.Sprintf("PING from %s", self.Username)
			sendMessageToPeer(username, msg)
		}
	}
}

// Entry point
func main() {
	// Register this peer with the bootstrap server
	data, err := json.Marshal(self)
	if err != nil {
		log.Fatalf("Error marshalling peer; %v", err)
	}

	endpoint := fmt.Sprintf("http://%v/register", serverAddress)
	_, err = http.Post(endpoint, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Fatalln("Failed to register with bootstrap server:", err)
	}

	getInitialPeers()

	go listenToWS()
	go startP2PListener()
	go pingKnownPeers()

	// Graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	fmt.Println("\n[EXIT] Shutting down.")
}

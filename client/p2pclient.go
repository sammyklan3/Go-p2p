package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"
)

type Peer struct {
	Username string    `json:"username"`
	IP       string    `json:"ip"`
	Port     string    `json:"port"`
	LastSeen time.Time `json:"lastSeen"`
}

var (
	peerList   = make(map[string]Peer)
	peerFile   = "clientPeers.json"
	self       Peer
	peerListMu sync.Mutex
)

func registerWithServer(serverURL, username, port string) error {
	localIP := getLocalIP()
	self = Peer{
		Username: username,
		IP:       localIP,
		Port:     port,
		LastSeen: time.Now(),
	}
	data, _ := json.Marshal(self)
	_, err := http.Post(serverURL+"/register", "application/json", bytes.NewBuffer(data))
	return err
}

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
			p.LastSeen = time.Now()
			peerListMu.Lock()
			peerList[p.Username] = p
			peerListMu.Unlock()
		}
	}
	savePeersToFile()
}

func startP2PUDPListener(port string) {
	addr := ":" + port
	udpAddr, _ := net.ResolveUDPAddr("udp", addr)
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatalf("Failed to bind to UDP port %s: %v\n", port, err)
	}
	defer conn.Close()
	fmt.Println("[LISTENING] UDP on", addr)

	buf := make([]byte, 2048)
	for {
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Println("UDP Read error:", err)
			continue
		}

		msg := string(buf[:n])
		if len(msg) > 8 && msg[:8] == "[GOSSIP]" {
			handleIncomingGossip(msg[8:])
		} else {
			fmt.Printf("[UDP] Received from %s: %s\n", remoteAddr, msg)
		}
	}
}

func handleIncomingGossip(data string) {
	var incoming map[string]Peer
	err := json.Unmarshal([]byte(data), &incoming)
	if err != nil {
		log.Println("[GOSSIP] Decode error:", err)
		return
	}

	peerListMu.Lock()
	defer peerListMu.Unlock()

	for username, peer := range incoming {
		if username == self.Username {
			continue
		}
		peer.LastSeen = time.Now()
		if _, exists := peerList[username]; !exists {
			fmt.Printf("[GOSSIP] New peer: %s (%s:%s)\n", peer.Username, peer.IP, peer.Port)
		}
		peerList[username] = peer
	}
	savePeersToFile()
}

func gossipSubsetPeers() {
	peerListMu.Lock()
	defer peerListMu.Unlock()

	// Collect peers into slice
	peers := make([]Peer, 0, len(peerList))
	for _, p := range peerList {
		peers = append(peers, p)
	}

	// Shuffle and pick a subset
	rand.Shuffle(len(peers), func(i, j int) { peers[i], peers[j] = peers[j], peers[i] })
	count := 3
	if len(peers) < count {
		count = len(peers)
	}
	selected := peers[:count]

	// Send gossip to each
	data, _ := json.Marshal(peerList)
	for _, peer := range selected {
		addr := net.JoinHostPort(peer.IP, peer.Port)
		udpAddr, _ := net.ResolveUDPAddr("udp", addr)
		conn, err := net.DialUDP("udp", nil, udpAddr)
		if err != nil {
			continue
		}
		conn.Write([]byte("[GOSSIP]" + string(data)))
		conn.Close()
	}
}

func pruneStalePeers() {
	ticker := time.NewTicker(1 * time.Minute)
	for range ticker.C {
		now := time.Now()
		peerListMu.Lock()
		for username, peer := range peerList {
			if now.Sub(peer.LastSeen) > 2*time.Minute {
				fmt.Println("[PRUNE] Removing stale peer:", username)
				delete(peerList, username)
			}
		}
		peerListMu.Unlock()
		savePeersToFile()
	}
}

func sendTCPMessage(username, message string) {
	peerListMu.Lock()
	peer, exists := peerList[username]
	peerListMu.Unlock()
	if !exists {
		fmt.Println("[TCP] Peer not found:", username)
		return
	}

	addr := net.JoinHostPort(peer.IP, peer.Port)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Println("[TCP] Dial error:", err)
		return
	}
	defer conn.Close()
	conn.Write([]byte(message))
	fmt.Printf("[TCP] Sent to %s: %s\n", username, message)
}

func startTCPListener(port string) {
	addr := ":" + port
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("TCP Listen error on %s: %v", addr, err)
	}
	defer ln.Close()
	fmt.Println("[LISTENING] TCP on", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("[TCP] Accept error:", err)
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			buf := make([]byte, 2048)
			n, _ := c.Read(buf)
			fmt.Printf("[TCP] Received: %s\n", string(buf[:n]))
		}(conn)
	}
}

func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

func savePeersToFile() {
	peerListMu.Lock()
	defer peerListMu.Unlock()

	data, err := json.MarshalIndent(peerList, "", "  ")
	if err != nil {
		log.Println("[SAVE] Marshal error:", err)
		return
	}
	err = os.WriteFile(peerFile, data, 0644)
	if err != nil {
		log.Println("[SAVE] Write error:", err)
	}
}

func loadPeersFromFile() {
	data, err := os.ReadFile(peerFile)
	if err != nil {
		log.Println("[LOAD] No previous peer file found.")
		return
	}
	var stored map[string]Peer
	err = json.Unmarshal(data, &stored)
	if err != nil {
		log.Println("[LOAD] Decode error:", err)
		return
	}

	peerListMu.Lock()
	for username, peer := range stored {
		if username != self.Username {
			peerList[username] = peer
		}
	}
	peerListMu.Unlock()
}

func startPeerLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		gossipSubsetPeers()
	}
}

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: go run main.go <username> <port> <bootstrap-host:port>")
		return
	}
	username := os.Args[1]
	port := os.Args[2]
	bootstrap := os.Args[3]

	err := registerWithServer("http://"+bootstrap, username, port)
	if err != nil {
		log.Fatalln("Failed to register with bootstrap:", err)
	}

	loadPeersFromFile()
	getInitialPeers("http://" + bootstrap)

	go startP2PUDPListener(port)
	go startTCPListener(port)
	go startPeerLoop()
	go pruneStalePeers()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	fmt.Println("\n[EXIT] Client shutting down.")
}

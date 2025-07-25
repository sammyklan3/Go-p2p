package p2p

import (
	"fmt"
	"math"
	"math/rand"
	"net"
	"strconv"
)

func ValidateIPAddress(address string) error {
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("invalid IP address")
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port number")
	}

	if port <= 0 || port >= 65535 {
		return fmt.Errorf("port number out of range 1 - 65535")
	}
	return nil
}

// Get local machine IP address
func GetLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

func GenerateRandomPort() string {
	port := rand.Intn(math.MaxInt16)
	if port == 0 {
		return GenerateRandomPort()
	}
	return fmt.Sprintf("%v", port)
}

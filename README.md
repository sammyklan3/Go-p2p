# 🕸️ Go-P2P: Peer-to-Peer Networking with Bootstrap Server

This project is a basic implementation of a Peer-to-Peer (P2P) network written in Go, using a central bootstrap server for initial peer discovery and WebSockets for real-time updates.

## 🚀 Features

- Peer registration via a bootstrap server
- Real-time peer discovery with WebSockets
- UDP-based peer-to-peer messaging
- Graceful peer disconnection
- Optional peer heartbeat monitoring (for peer availability)

---

## 📦 Project Structure

- **Bootstrap Server**: Maintains a list of active peers and provides:
  - `/register` endpoint for new peers to join
  - `/peers` endpoint for fetching all peers
  - `/ws` WebSocket endpoint for real-time updates

- **P2P Client**:
  - Registers with the server
  - Listens for new peers via WebSocket
  - Sends and receives messages using UDP

---

## 🔧 Usage

### Start the P2P node:

```bash
go run client/p2pclient.go <username> <udp-port> <bootstrap-host:port>

## Example:
```bash
go run main.go alice 10000 localhost:8080
package p2p

type Peer struct {
	Username string `json:"username"`
	IP       string `json:"ip"`
	Port     string `json:"port"`
}

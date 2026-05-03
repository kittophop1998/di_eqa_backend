package port

// Event is the payload broadcast over WebSocket rooms.
type Event struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// EventPort — port for real-time event broadcasting (driven).
type EventPort interface {
	Broadcast(room string, event Event)
}

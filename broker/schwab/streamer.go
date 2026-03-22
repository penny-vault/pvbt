package schwab

import "github.com/penny-vault/pvbt/broker"

// fillStreamer delivers real-time fill events via a WebSocket connection to
// Schwab's account activity streamer. Full implementation is added in a
// later task.
type fillStreamer struct {
	info    schwabStreamerInfo
	account string
	fills   chan<- broker.Fill
}

// connect establishes the WebSocket connection and subscribes to account
// activity events. Called during Connect (implemented later).
func (fs *fillStreamer) connect() error {
	return nil
}

// disconnect closes the WebSocket connection and stops the event loop.
// Called during Disconnect (implemented later).
func (fs *fillStreamer) disconnect() {
}

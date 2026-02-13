package aiagent

import (
	"sync"

	"github.com/cloudwego/eino/schema"
)

// simpleMemoryStore holds all session memories keyed by session ID.
var simpleMemoryStore = make(map[string]*SimpleMemory)
var memMu sync.Mutex

// GetSimpleMemory returns an existing memory for the given ID, or creates a new one.
func GetSimpleMemory(id string) *SimpleMemory {
	memMu.Lock()
	defer memMu.Unlock()
	if mem, ok := simpleMemoryStore[id]; ok {
		return mem
	}
	newMem := &SimpleMemory{
		ID:            id,
		Messages:      []*schema.Message{},
		MaxWindowSize: DefaultMaxWindowSize,
	}
	simpleMemoryStore[id] = newMem
	return newMem
}

// DeleteMemory removes a session memory by ID.
func DeleteMemory(id string) {
	memMu.Lock()
	defer memMu.Unlock()
	delete(simpleMemoryStore, id)
}

// SimpleMemory implements a sliding-window conversation memory.
type SimpleMemory struct {
	ID            string            `json:"id"`
	Messages      []*schema.Message `json:"messages"`
	MaxWindowSize int
	mu            sync.Mutex
}

// SetMessages appends a message and trims the history to keep within the window size.
func (m *SimpleMemory) SetMessages(msg *schema.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Messages = append(m.Messages, msg)
	if len(m.Messages) > m.MaxWindowSize {
		// Ensure messages are discarded in pairs to maintain conversation pairing
		excess := len(m.Messages) - m.MaxWindowSize
		if excess%2 != 0 {
			excess++
		}
		m.Messages = m.Messages[excess:]
	}
}

// GetMessages returns the current conversation history.
func (m *SimpleMemory) GetMessages() []*schema.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Messages
}

// ClearMessages resets the conversation memory.
func (m *SimpleMemory) ClearMessages() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Messages = []*schema.Message{}
}

package live

import (
	"sync"
	"time"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
)

const (
	DefaultMaxStreams  = 128
	DefaultStreamBytes = 64 << 10
)

type Status string

const (
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

type Snapshot struct {
	AgentID     id.AgentID `json:"agent_id"`
	Status      Status     `json:"status"`
	StartOffset int64      `json:"start_offset"`
	EndOffset   int64      `json:"end_offset"`
	Data        string     `json:"data"`
	ResponseRef string     `json:"response_ref,omitempty"`
	Error       string     `json:"error,omitempty"`
}

type stream struct {
	createdAt   time.Time
	status      Status
	startOffset int64
	endOffset   int64
	data        []byte
	responseRef string
	err         string
}

type Bus struct {
	mu         sync.RWMutex
	maxStreams int
	maxBytes   int
	streams    map[id.AgentID]*stream
}

func New(maxStreams, maxBytes int) *Bus {
	if maxStreams <= 0 {
		maxStreams = DefaultMaxStreams
	}
	if maxBytes <= 0 {
		maxBytes = DefaultStreamBytes
	}
	return &Bus{
		maxStreams: maxStreams,
		maxBytes:   maxBytes,
		streams:    make(map[id.AgentID]*stream),
	}
}

func (b *Bus) Begin(agentID id.AgentID, now time.Time) error {
	if agentID == "" {
		return errs.New(errs.CodeInvalidArgument, "live.begin", "agent id is required")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.streams[agentID]; exists {
		b.streams[agentID] = &stream{createdAt: now.UTC(), status: StatusRunning}
		return nil
	}

	if len(b.streams) >= b.maxStreams {
		var oldestID id.AgentID
		var oldest time.Time
		for candidateID, candidate := range b.streams {
			if candidate.status == StatusRunning {
				continue
			}
			if oldestID == "" || candidate.createdAt.Before(oldest) {
				oldestID = candidateID
				oldest = candidate.createdAt
			}
		}
		if oldestID == "" {
			return errs.New(errs.CodeResourceExhausted, "live.begin", "all live stream slots are active")
		}
		delete(b.streams, oldestID)
	}

	b.streams[agentID] = &stream{createdAt: now.UTC(), status: StatusRunning}
	return nil
}

func (b *Bus) Publish(agentID id.AgentID, text string) {
	if text == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	current := b.streams[agentID]
	if current == nil || current.status != StatusRunning {
		return
	}

	chunk := []byte(text)
	current.data = append(current.data, chunk...)
	current.endOffset += int64(len(chunk))

	if len(current.data) > b.maxBytes {
		over := len(current.data) - b.maxBytes
		copy(current.data, current.data[over:])
		current.data = current.data[:b.maxBytes]
		current.startOffset += int64(over)
	}
}

func (b *Bus) Complete(agentID id.AgentID, responseRef string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if current := b.streams[agentID]; current != nil {
		current.status = StatusCompleted
		current.responseRef = responseRef
	}
}

func (b *Bus) Fail(agentID id.AgentID, message string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if current := b.streams[agentID]; current != nil {
		current.status = StatusFailed
		current.err = message
	}
}

func (b *Bus) Snapshot(agentID id.AgentID) (Snapshot, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	current := b.streams[agentID]
	if current == nil {
		return Snapshot{}, false
	}
	return Snapshot{
		AgentID:     agentID,
		Status:      current.status,
		StartOffset: current.startOffset,
		EndOffset:   current.endOffset,
		Data:        string(append([]byte(nil), current.data...)),
		ResponseRef: current.responseRef,
		Error:       current.err,
	}, true
}

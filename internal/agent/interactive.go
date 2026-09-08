package agent

import (
	"context"
	"strings"
	"sync"

	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/ledger"
	"github.com/Methamorphe/go-agent/internal/process"
)

const (
	MaxInteractiveMessages = 64
	MaxInteractiveBytes    = 256 << 10
	MaxInteractiveText     = 64 << 10
)

type MessageQueue string

const (
	QueueSteer    MessageQueue = "steer"
	QueueFollowUp MessageQueue = "follow_up"
)

type SendMessageRequest struct {
	AgentID id.AgentID   `json:"agent_id"`
	Text    string       `json:"text"`
	Queue   MessageQueue `json:"queue"`
}

type SendMessageResult struct {
	MessageID id.MessageID `json:"message_id"`
	AgentID   id.AgentID   `json:"agent_id"`
	Queue     MessageQueue `json:"queue"`
	Pending   int          `json:"pending"`
}

type queuedMessage struct {
	id    id.MessageID
	text  string
	queue MessageQueue
}

type interactiveSession struct {
	mu        sync.Mutex
	accepting bool
	steer     []queuedMessage
	followUp  []queuedMessage
	bytes     int
}

func (r *Runner) SendMessage(_ context.Context, request SendMessageRequest) (SendMessageResult, error) {
	if r == nil || request.AgentID == "" {
		return SendMessageResult{}, errs.New(errs.CodeInvalidArgument, "agent.message", "agent id is required")
	}
	text := strings.TrimSpace(request.Text)
	if text == "" {
		return SendMessageResult{}, errs.New(errs.CodeInvalidArgument, "agent.message", "message text is required")
	}
	if len(text) > MaxInteractiveText {
		return SendMessageResult{}, errs.New(errs.CodeResourceExhausted, "agent.message", "message exceeds maximum size")
	}
	if request.Queue == "" {
		request.Queue = QueueSteer
	}
	if request.Queue != QueueSteer && request.Queue != QueueFollowUp {
		return SendMessageResult{}, errs.New(errs.CodeInvalidArgument, "agent.message", "queue must be steer or follow_up")
	}
	messageID, err := r.ids.Message()
	if err != nil {
		return SendMessageResult{}, errs.Wrap(errs.CodeInternal, "agent.message", "generate message id", err)
	}

	r.sessionsMu.Lock()
	session := r.sessions[request.AgentID]
	r.sessionsMu.Unlock()
	if session == nil {
		return SendMessageResult{}, errs.New(errs.CodeConflict, "agent.message", "agent is not actively running")
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	if !session.accepting {
		return SendMessageResult{}, errs.New(errs.CodeConflict, "agent.message", "agent is completing and no longer accepts input")
	}
	pending := len(session.steer) + len(session.followUp)
	if pending >= MaxInteractiveMessages || session.bytes+len(text) > MaxInteractiveBytes {
		return SendMessageResult{}, errs.New(errs.CodeResourceExhausted, "agent.message", "interactive input queue is full")
	}
	message := queuedMessage{id: messageID, text: text, queue: request.Queue}
	if request.Queue == QueueFollowUp {
		session.followUp = append(session.followUp, message)
	} else {
		session.steer = append(session.steer, message)
	}
	session.bytes += len(text)
	return SendMessageResult{MessageID: messageID, AgentID: request.AgentID, Queue: request.Queue, Pending: pending + 1}, nil
}

func (r *Runner) openInteractiveSession(agentID id.AgentID) (*interactiveSession, error) {
	r.sessionsMu.Lock()
	defer r.sessionsMu.Unlock()
	if _, exists := r.sessions[agentID]; exists {
		return nil, errs.New(errs.CodeConflict, "agent.run", "agent already has an active interactive session")
	}
	session := &interactiveSession{accepting: true}
	r.sessions[agentID] = session
	return session, nil
}

func (r *Runner) closeInteractiveSession(agentID id.AgentID, session *interactiveSession) {
	if session != nil {
		session.mu.Lock()
		session.accepting = false
		session.mu.Unlock()
	}
	r.sessionsMu.Lock()
	if r.sessions[agentID] == session {
		delete(r.sessions, agentID)
	}
	r.sessionsMu.Unlock()
}

func (r *Runner) drainSteering(ctx context.Context, session *interactiveSession, state process.State, meta process.CommandMeta) (process.State, []string, error) {
	if session == nil {
		return state, nil, nil
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	return r.recordQueuedLocked(ctx, session, state, meta, false)
}

func (r *Runner) finalContinuation(ctx context.Context, session *interactiveSession, state process.State, meta process.CommandMeta) (process.State, []string, bool, error) {
	if session == nil {
		return state, nil, false, nil
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if len(session.steer) == 0 && len(session.followUp) == 0 {
		session.accepting = false
		return state, nil, false, nil
	}
	next, messages, err := r.recordQueuedLocked(ctx, session, state, meta, true)
	if err != nil {
		return state, nil, false, err
	}
	return next, messages, true, nil
}

func (r *Runner) recordQueuedLocked(ctx context.Context, session *interactiveSession, state process.State, meta process.CommandMeta, includeFollowUp bool) (process.State, []string, error) {
	messages := append([]queuedMessage(nil), session.steer...)
	if includeFollowUp {
		messages = append(messages, session.followUp...)
	}
	if len(messages) == 0 {
		return state, nil, nil
	}
	userMeta := meta
	userMeta.Actor = ledger.ActorRef{Kind: ledger.ActorUser, ID: "interactive-client"}
	texts := make([]string, 0, len(messages))
	for _, message := range messages {
		expected := state.Version
		next, err := r.processes.UserMessageReceived(ctx, state.AgentID, &expected, process.UserMessageReceivedPayload{
			MessageID: message.id,
			Text:      message.text,
			Queue:     string(message.queue),
		}, userMeta)
		if err != nil {
			return state, nil, err
		}
		state = next
		texts = append(texts, message.text)
	}
	for _, message := range session.steer {
		session.bytes -= len(message.text)
	}
	session.steer = nil
	if includeFollowUp {
		for _, message := range session.followUp {
			session.bytes -= len(message.text)
		}
		session.followUp = nil
	}
	if session.bytes < 0 {
		session.bytes = 0
	}
	return state, texts, nil
}

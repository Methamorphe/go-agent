package gui

import (
	"context"
	"sync"
	"time"

	"github.com/Methamorphe/go-agent/internal/agent"
	controlapi "github.com/Methamorphe/go-agent/internal/control/api"
	"github.com/Methamorphe/go-agent/internal/errs"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/live"
	agentprocess "github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/workspace"
)

const BridgeVersion uint32 = 1

const (
	defaultReadTimeout     = 5 * time.Second
	defaultMutationTimeout = 30 * time.Second
	maxMutationTimeout     = 2 * time.Minute
)

// Caller is the small transport boundary shared with the daemon control client.
// Desktop code never reads SQLite or bypasses runtime authorization.
type Caller interface {
	Call(context.Context, string, any, any) error
}

// LaunchOptions are presentation-only defaults supplied by the desktop binary.
type LaunchOptions struct {
	AgentID id.AgentID     `json:"agent_id,omitempty"`
	Mode    workspace.Mode `json:"mode,omitempty"`
}

// Bootstrap describes stable bridge capabilities without copying canonical state.
type Bootstrap struct {
	BridgeVersion            uint32           `json:"bridge_version"`
	WorkspaceProtocolVersion uint32           `json:"workspace_protocol_version"`
	Launch                    LaunchOptions    `json:"launch"`
	Modes                     []workspace.Mode `json:"modes"`
	MaxHistoryPage           int              `json:"max_history_page"`
	MaxTree                  int              `json:"max_tree"`
	RecommendedRefreshMS     int              `json:"recommended_refresh_ms"`
	RecommendedLiveRefreshMS int              `json:"recommended_live_refresh_ms"`
}

// ProcessTransition is a versioned suspend/resume request.
type ProcessTransition struct {
	AgentID         id.AgentID `json:"agent_id"`
	ExpectedVersion uint64     `json:"expected_version"`
	Reason          string     `json:"reason,omitempty"`
}

// StartupProbe is presentation-only instrumentation used by the cross-platform
// Wails smoke gate. It never influences daemon state or runtime policy.
type StartupProbe struct {
	once  sync.Once
	ready chan time.Time
}

func NewStartupProbe() *StartupProbe {
	return &StartupProbe{ready: make(chan time.Time, 1)}
}

func (p *StartupProbe) markReady(at time.Time) {
	if p == nil {
		return
	}
	p.once.Do(func() {
		p.ready <- at.UTC()
		close(p.ready)
	})
}

// Ready exposes the one-shot frontend-ready signal to the desktop host. The
// probe itself is not registered as a Wails service.
func (p *StartupProbe) Ready() <-chan time.Time {
	if p == nil {
		return nil
	}
	return p.ready
}

// DesktopService is the only Go service exposed to Wails. It is intentionally a
// thin adapter over the local daemon protocol: durable state and policy stay in
// the daemon and closing the desktop process cannot manufacture a runtime
// transition.
type DesktopService struct {
	caller          Caller
	launch          LaunchOptions
	readTimeout     time.Duration
	mutationTimeout time.Duration
	startup         *StartupProbe
}

func NewService(caller Caller, launch LaunchOptions) *DesktopService {
	return NewServiceWithStartupProbe(caller, launch, NewStartupProbe())
}

func NewServiceWithStartupProbe(caller Caller, launch LaunchOptions, startup *StartupProbe) *DesktopService {
	if !launch.Mode.Valid() {
		launch.Mode = workspace.ModeAct
	}
	if startup == nil {
		startup = NewStartupProbe()
	}
	return &DesktopService{
		caller:          caller,
		launch:          launch,
		readTimeout:     defaultReadTimeout,
		mutationTimeout: defaultMutationTimeout,
		startup:         startup,
	}
}

func (s *DesktopService) Bootstrap() Bootstrap {
	launch := s.launch
	if !launch.Mode.Valid() {
		launch.Mode = workspace.ModeAct
	}
	return Bootstrap{
		BridgeVersion:            BridgeVersion,
		WorkspaceProtocolVersion: workspace.ProtocolVersion,
		Launch:                    launch,
		Modes: []workspace.Mode{
			workspace.ModeAsk,
			workspace.ModePlan,
			workspace.ModeAct,
			workspace.ModeReview,
			workspace.ModeObserve,
		},
		MaxHistoryPage:           workspace.MaxPageSize,
		MaxTree:                  workspace.MaxTreeLimit,
		RecommendedRefreshMS:     160,
		RecommendedLiveRefreshMS: 80,
	}
}

// FrontendReady is called once by the mounted TypeScript workspace. It exists
// only to prove that the native window, WebView, bundled assets and generated
// bindings completed their startup path during smoke tests.
func (s *DesktopService) FrontendReady() {
	if s == nil {
		return
	}
	s.startup.markReady(time.Now())
}

func (s *DesktopService) call(messageType string, request, response any, timeout time.Duration) error {
	if s == nil || s.caller == nil {
		return errs.New(errs.CodeUnavailable, "gui.bridge", "runtime control client is unavailable")
	}
	if timeout <= 0 {
		timeout = defaultReadTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return s.caller.Call(ctx, messageType, request, response)
}

func (s *DesktopService) Attach(request workspace.AttachRequest) (workspace.Snapshot, error) {
	if request.AgentID == "" {
		request.AgentID = s.launch.AgentID
	}
	if request.Mode == "" {
		request.Mode = s.launch.Mode
	}
	if !request.Mode.Valid() {
		request.Mode = workspace.ModeAct
	}
	var response controlapi.WorkspaceAttachResponse
	err := s.call(controlapi.MessageWorkspaceAttach, request, &response, s.readTimeout)
	return response.Snapshot, err
}

func (s *DesktopService) Refresh(request workspace.RefreshRequest) (workspace.Refresh, error) {
	var response controlapi.WorkspaceRefreshResponse
	err := s.call(controlapi.MessageWorkspaceRefresh, request, &response, s.readTimeout)
	return response.Refresh, err
}

func (s *DesktopService) History(request workspace.HistoryRequest) (workspace.ConversationViewport, error) {
	var response controlapi.WorkspaceHistoryResponse
	err := s.call(controlapi.MessageWorkspaceHistory, request, &response, s.readTimeout)
	return response.Viewport, err
}

func (s *DesktopService) Search(request workspace.SearchRequest) (workspace.SearchResult, error) {
	var response controlapi.WorkspaceSearchResponse
	err := s.call(controlapi.MessageWorkspaceSearch, request, &response, s.readTimeout)
	return response.Result, err
}

func (s *DesktopService) Live(agentID id.AgentID) (live.Snapshot, error) {
	var response controlapi.AgentLiveResponse
	err := s.call(controlapi.MessageAgentLive, controlapi.AgentLiveRequest{AgentID: agentID}, &response, s.readTimeout)
	return response.Stream, err
}

func (s *DesktopService) SendMessage(request agent.SendMessageRequest) (agent.SendMessageResult, error) {
	var response controlapi.AgentMessageResponse
	err := s.call(controlapi.MessageAgentMessage, request, &response, s.mutationTimeout)
	return response.Result, err
}

func (s *DesktopService) Suspend(request ProcessTransition) (agentprocess.State, error) {
	var response controlapi.ProcessResponse
	expected := request.ExpectedVersion
	err := s.call(controlapi.MessageProcessSuspend, controlapi.ProcessTransitionRequest{
		AgentID: request.AgentID, ExpectedVersion: &expected, Reason: request.Reason,
	}, &response, s.mutationTimeout)
	return response.Process, err
}

func (s *DesktopService) Resume(request ProcessTransition) (agentprocess.State, error) {
	var response controlapi.ProcessResponse
	expected := request.ExpectedVersion
	err := s.call(controlapi.MessageProcessResume, controlapi.ProcessTransitionRequest{
		AgentID: request.AgentID, ExpectedVersion: &expected, Reason: request.Reason,
	}, &response, s.mutationTimeout)
	return response.Process, err
}

func (s *DesktopService) OperateTransaction(request controlapi.WorkspaceTransactionOperateRequest) (controlapi.WorkspaceTransactionOperateResponse, error) {
	timeout := s.mutationTimeout
	if request.TimeoutMS > 0 {
		candidate := time.Duration(request.TimeoutMS)*time.Millisecond + 5*time.Second
		if candidate > timeout {
			timeout = candidate
		}
	}
	if timeout > maxMutationTimeout {
		timeout = maxMutationTimeout
	}
	var response controlapi.WorkspaceTransactionOperateResponse
	err := s.call(controlapi.MessageWorkspaceTransactionOperate, request, &response, timeout)
	return response, err
}

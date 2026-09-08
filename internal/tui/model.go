package tui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/Methamorphe/go-agent/internal/agent"
	"github.com/Methamorphe/go-agent/internal/id"
	"github.com/Methamorphe/go-agent/internal/process"
	"github.com/Methamorphe/go-agent/internal/workspace"
)

type runtimeClient interface {
	Attach(context.Context, workspace.AttachRequest) (workspace.Snapshot, error)
	Refresh(context.Context, workspace.RefreshRequest) (workspace.Refresh, error)
	History(context.Context, workspace.HistoryRequest) (workspace.ConversationViewport, error)
	Search(context.Context, workspace.SearchRequest) (workspace.SearchResult, error)
	SendMessage(context.Context, id.AgentID, string, agent.MessageQueue) (agent.SendMessageResult, error)
	Suspend(context.Context, id.AgentID, uint64, string) (process.State, error)
	Resume(context.Context, id.AgentID, uint64, string) (process.State, error)
}

type overlay int

const (
	overlayNone overlay = iota
	overlayPalette
	overlaySearch
	overlayHelp
)

type inspectorTab int

const (
	inspectorRuntime inspectorTab = iota
	inspectorTransactions
	inspectorForks
	inspectorContext
	inspectorTeams
	inspectorImprovements
	inspectorCount
)

type Model struct {
	ctx    context.Context
	client runtimeClient
	cfg    Config

	width  int
	height int

	rootID  id.AgentID
	focusID id.AgentID
	mode    workspace.Mode
	cursor  uint64

	tree       []workspace.ProcessSummary
	blocks     []workspace.Block
	inspector  workspace.Inspector
	oldest     uint64
	hasPrevious bool

	selectedAgent int
	scroll        int
	inspectorTab  inspectorTab

	composer        string
	composerFocused bool
	overlay         overlay
	overlayInput    string
	searchResults   []workspace.Block

	refreshCount int
	loading      bool
	status       string
	err          error
}

type attachMsg struct {
	snapshot workspace.Snapshot
	err      error
}

type refreshMsg struct {
	refresh workspace.Refresh
	err     error
}

type historyMsg struct {
	viewport workspace.ConversationViewport
	err      error
}

type searchMsg struct {
	result workspace.SearchResult
	err    error
}

type mutationMsg struct {
	state process.State
	label string
	err   error
}

type sendMsg struct {
	result agent.SendMessageResult
	queue  agent.MessageQueue
	err    error
}

type tickMsg time.Time

func NewModel(ctx context.Context, client runtimeClient, cfg Config) Model {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg = cfg.normalized()
	return Model{
		ctx: ctx,
		client: client,
		cfg: cfg,
		focusID: cfg.AgentID,
		mode: cfg.Mode,
		composerFocused: true,
		status: "connecting to agent runtime…",
		loading: true,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.attachCmd(), m.tickCmd())
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyPressMsg:
		return m.updateKey(msg)

	case attachMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.status = "runtime unavailable — ctrl+p for commands, ctrl+c to quit"
			return m, nil
		}
		m.applySnapshot(msg.snapshot)
		m.status = "attached"
		m.err = nil
		return m, nil

	case refreshMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.status = "refresh failed"
			return m, nil
		}
		if msg.refresh.Behind {
			m.status = "catching up…"
			m.loading = true
			return m, m.attachCmd()
		}
		m.applyRefresh(msg.refresh)
		m.err = nil
		return m, nil

	case historyMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.status = "history load failed"
			return m, nil
		}
		m.prependBlocks(msg.viewport.Blocks)
		m.oldest = msg.viewport.Oldest
		m.hasPrevious = msg.viewport.HasPrevious
		m.status = fmt.Sprintf("loaded %d older blocks", len(msg.viewport.Blocks))
		m.err = nil
		return m, nil

	case searchMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.status = "search failed"
			return m, nil
		}
		m.searchResults = append(m.searchResults[:0], msg.result.Blocks...)
		m.status = fmt.Sprintf("%d search results", len(m.searchResults))
		m.err = nil
		return m, nil

	case sendMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.status = "message rejected"
			return m, nil
		}
		m.status = fmt.Sprintf("%s queued · %d pending", msg.queue, msg.result.Pending)
		m.err = nil
		return m, m.refreshCmd(true)

	case mutationMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			m.status = msg.label + " failed"
			return m, nil
		}
		m.patchState(msg.state)
		m.status = msg.label
		m.err = nil
		return m, m.refreshCmd(true)

	case tickMsg:
		cmds := []tea.Cmd{m.tickCmd()}
		if m.rootID != "" && !m.loading {
			m.refreshCount++
			inspect := m.refreshCount%m.cfg.InspectorEvery == 0
			cmds = append(cmds, m.refreshCmd(inspect))
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m Model) updateKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	if key == "ctrl+p" {
		m.overlay = overlayPalette
		m.overlayInput = ""
		m.composerFocused = false
		return m, nil
	}
	if m.overlay != overlayNone {
		return m.updateOverlayKey(msg)
	}

	if m.composerFocused {
		switch key {
		case "esc":
			m.composerFocused = false
			return m, nil
		case "enter":
			return m.sendComposer(agent.QueueSteer)
		case "alt+enter", "ctrl+j":
			return m.sendComposer(agent.QueueFollowUp)
		case "backspace":
			m.composer = trimLastRune(m.composer)
			return m, nil
		case "ctrl+u":
			m.composer = ""
			return m, nil
		case "ctrl+1", "ctrl+2", "ctrl+3", "ctrl+4", "ctrl+5":
			m.setModeKey(key)
			return m, nil
		}
		if text := msg.Key().Text; text != "" {
			m.composer += text
		}
		return m, nil
	}

	switch key {
	case "q":
		return m, tea.Quit
	case "i", "enter":
		m.composerFocused = true
	case "?":
		m.overlay = overlayHelp
	case "/":
		m.overlay = overlaySearch
		m.overlayInput = ""
	case "up", "k":
		m.selectAgent(-1)
	case "down", "j":
		m.selectAgent(1)
	case "pgup", "u":
		if m.hasPrevious && !m.loading {
			m.loading = true
			return m, m.historyCmd()
		}
	case "pgdown", "d":
		m.scroll = max(0, m.scroll-10)
	case "home", "g":
		m.scroll = len(m.blocks)
	case "end", "G":
		m.scroll = 0
	case "[":
		m.inspectorTab = (m.inspectorTab + inspectorCount - 1) % inspectorCount
	case "]":
		m.inspectorTab = (m.inspectorTab + 1) % inspectorCount
	case "1":
		m.mode = workspace.ModeAsk
	case "2":
		m.mode = workspace.ModePlan
	case "3":
		m.mode = workspace.ModeAct
	case "4":
		m.mode = workspace.ModeReview
	case "5":
		m.mode = workspace.ModeObserve
	case "s":
		return m.toggleSuspend()
	case "r":
		if !m.loading {
			m.loading = true
			return m, m.attachCmd()
		}
	}
	return m, nil
}

func (m Model) updateOverlayKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.overlay = overlayNone
		m.overlayInput = ""
		return m, nil
	case "backspace":
		m.overlayInput = trimLastRune(m.overlayInput)
		return m, nil
	case "enter":
		switch m.overlay {
		case overlayPalette:
			return m.executePalette(strings.TrimSpace(m.overlayInput))
		case overlaySearch:
			query := strings.TrimSpace(m.overlayInput)
			m.overlay = overlayNone
			m.overlayInput = ""
			if query == "" || m.rootID == "" {
				return m, nil
			}
			m.loading = true
			return m, m.searchCmd(query)
		case overlayHelp:
			m.overlay = overlayNone
			return m, nil
		}
	}
	if m.overlay == overlayHelp {
		return m, nil
	}
	if text := msg.Key().Text; text != "" {
		m.overlayInput += text
	}
	return m, nil
}

func (m Model) executePalette(command string) (tea.Model, tea.Cmd) {
	m.overlay = overlayNone
	m.overlayInput = ""
	fields := strings.Fields(strings.ToLower(command))
	if len(fields) == 0 {
		return m, nil
	}
	switch fields[0] {
	case "ask", "plan", "act", "review", "observe":
		m.mode = workspace.Mode(strings.ToUpper(fields[0]))
		m.status = "mode " + string(m.mode)
	case "theme":
		if len(fields) > 1 && fields[1] == "light" {
			m.cfg.Theme = LightTheme()
		} else {
			m.cfg.Theme = DarkTheme()
		}
	case "agent":
		if len(fields) > 1 && fields[1] == "prev" {
			m.selectAgent(-1)
		} else {
			m.selectAgent(1)
		}
	case "older", "history":
		if m.hasPrevious && !m.loading {
			m.loading = true
			return m, m.historyCmd()
		}
	case "search":
		m.overlay = overlaySearch
	case "suspend", "resume":
		return m.toggleSuspend()
	case "transactions":
		m.inspectorTab = inspectorTransactions
	case "forks":
		m.inspectorTab = inspectorForks
	case "context", "mmu":
		m.inspectorTab = inspectorContext
	case "teams":
		m.inspectorTab = inspectorTeams
	case "improvements":
		m.inspectorTab = inspectorImprovements
	case "help":
		m.overlay = overlayHelp
	case "refresh":
		m.loading = true
		return m, m.attachCmd()
	}
	return m, nil
}

func (m Model) sendComposer(queue agent.MessageQueue) (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.composer)
	if text == "" || m.focusID == "" || m.loading {
		return m, nil
	}
	m.composer = ""
	m.loading = true
	m.status = "queueing " + string(queue) + "…"
	return m, m.sendCmd(text, queue)
}

func (m Model) toggleSuspend() (tea.Model, tea.Cmd) {
	current, ok := m.focusedSummary()
	if !ok || m.loading {
		return m, nil
	}
	m.loading = true
	if current.Status == process.StatusSuspended {
		m.status = "resuming…"
		return m, m.resumeCmd(current)
	}
	if current.Status == process.StatusReady || current.Status == process.StatusRunning || current.Status == process.StatusWaiting || current.Status == process.StatusSleeping {
		m.status = "suspending…"
		return m, m.suspendCmd(current)
	}
	m.loading = false
	m.status = "selected process cannot be suspended"
	return m, nil
}

func (m *Model) applySnapshot(snapshot workspace.Snapshot) {
	m.rootID = snapshot.RootAgentID
	m.focusID = snapshot.FocusedAgentID
	m.mode = snapshot.Mode
	m.cursor = snapshot.Cursor
	m.tree = append(m.tree[:0], snapshot.Tree...)
	m.blocks = boundedBlocks(append([]workspace.Block(nil), snapshot.Viewport.Blocks...), m.cfg.MaxCachedBlocks)
	m.inspector = snapshot.Inspector
	m.oldest = snapshot.Viewport.Oldest
	m.hasPrevious = snapshot.Viewport.HasPrevious
	m.selectedAgent = indexAgent(m.tree, m.focusID)
	m.scroll = 0
}

func (m *Model) applyRefresh(refresh workspace.Refresh) {
	m.cursor = refresh.Cursor
	for _, patch := range refresh.TreePatch {
		index := indexAgent(m.tree, patch.AgentID)
		if index >= 0 {
			m.tree[index] = patch
		} else {
			m.tree = append(m.tree, patch)
		}
	}
	if len(refresh.Blocks) > 0 {
		m.blocks = boundedBlocks(append(m.blocks, refresh.Blocks...), m.cfg.MaxCachedBlocks)
	}
	if refresh.Inspector != nil {
		m.inspector = *refresh.Inspector
	}
}

func (m *Model) prependBlocks(blocks []workspace.Block) {
	if len(blocks) == 0 {
		return
	}
	combined := make([]workspace.Block, 0, len(blocks)+len(m.blocks))
	combined = append(combined, blocks...)
	combined = append(combined, m.blocks...)
	m.blocks = boundedBlocks(combined, m.cfg.MaxCachedBlocks)
}

func (m *Model) patchState(state process.State) {
	index := indexAgent(m.tree, state.AgentID)
	if index < 0 {
		return
	}
	m.tree[index].Status = state.Status
	m.tree[index].Version = state.Version
	m.tree[index].UpdatedAt = state.UpdatedAt
}

func (m *Model) selectAgent(delta int) {
	if len(m.tree) == 0 {
		return
	}
	m.selectedAgent = (m.selectedAgent + delta + len(m.tree)) % len(m.tree)
	selected := m.tree[m.selectedAgent]
	if selected.AgentID == m.focusID {
		return
	}
	m.focusID = selected.AgentID
	m.cfg.AgentID = selected.AgentID
	m.loading = true
	m.status = "attaching " + shortID(selected.AgentID.String()) + "…"
}

func (m Model) focusedSummary() (workspace.ProcessSummary, bool) {
	index := indexAgent(m.tree, m.focusID)
	if index < 0 {
		return workspace.ProcessSummary{}, false
	}
	return m.tree[index], true
}

func indexAgent(tree []workspace.ProcessSummary, agentID id.AgentID) int {
	for index := range tree {
		if tree[index].AgentID == agentID {
			return index
		}
	}
	return -1
}

func boundedBlocks(blocks []workspace.Block, limit int) []workspace.Block {
	if limit <= 0 {
		limit = DefaultCacheBlocks
	}
	if len(blocks) <= limit {
		return blocks
	}
	copyOfTail := make([]workspace.Block, limit)
	copy(copyOfTail, blocks[len(blocks)-limit:])
	return copyOfTail
}

func (m Model) attachCmd() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return attachMsg{err: context.Canceled}
		}
		snapshot, err := m.client.Attach(m.ctx, workspace.AttachRequest{
			AgentID: m.focusID,
			Mode: m.mode,
			HistoryLimit: m.cfg.HistoryPageSize,
			TreeLimit: m.cfg.TreeLimit,
		})
		return attachMsg{snapshot: snapshot, err: err}
	}
}

func (m Model) refreshCmd(inspector bool) tea.Cmd {
	rootID, focusID, cursor := m.rootID, m.focusID, m.cursor
	return func() tea.Msg {
		refresh, err := m.client.Refresh(m.ctx, workspace.RefreshRequest{
			RootAgentID: rootID,
			FocusedAgentID: focusID,
			AfterSequence: cursor,
			Limit: m.cfg.HistoryPageSize,
			Inspector: inspector,
		})
		return refreshMsg{refresh: refresh, err: err}
	}
}

func (m Model) historyCmd() tea.Cmd {
	focusID, before := m.focusID, m.oldest
	return func() tea.Msg {
		viewport, err := m.client.History(m.ctx, workspace.HistoryRequest{
			AgentID: focusID,
			Before: before,
			Limit: m.cfg.HistoryPageSize,
		})
		return historyMsg{viewport: viewport, err: err}
	}
}

func (m Model) searchCmd(query string) tea.Cmd {
	rootID := m.rootID
	return func() tea.Msg {
		result, err := m.client.Search(m.ctx, workspace.SearchRequest{RootAgentID: rootID, Query: query, Limit: 100})
		return searchMsg{result: result, err: err}
	}
}

func (m Model) sendCmd(text string, queue agent.MessageQueue) tea.Cmd {
	focusID := m.focusID
	return func() tea.Msg {
		result, err := m.client.SendMessage(m.ctx, focusID, text, queue)
		return sendMsg{result: result, queue: queue, err: err}
	}
}

func (m Model) suspendCmd(summary workspace.ProcessSummary) tea.Cmd {
	return func() tea.Msg {
		state, err := m.client.Suspend(m.ctx, summary.AgentID, summary.Version, "tui_operator_suspend")
		return mutationMsg{state: state, label: "suspended", err: err}
	}
}

func (m Model) resumeCmd(summary workspace.ProcessSummary) tea.Cmd {
	return func() tea.Msg {
		state, err := m.client.Resume(m.ctx, summary.AgentID, summary.Version, "tui_operator_resume")
		return mutationMsg{state: state, label: "resumed", err: err}
	}
}

func (m Model) tickCmd() tea.Cmd {
	interval := m.cfg.RefreshInterval
	return tea.Tick(interval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) setModeKey(key string) {
	switch key {
	case "ctrl+1": m.mode = workspace.ModeAsk
	case "ctrl+2": m.mode = workspace.ModePlan
	case "ctrl+3": m.mode = workspace.ModeAct
	case "ctrl+4": m.mode = workspace.ModeReview
	case "ctrl+5": m.mode = workspace.ModeObserve
	}
}

func trimLastRune(value string) string {
	if value == "" {
		return value
	}
	_, size := utf8.DecodeLastRuneInString(value)
	return value[:len(value)-size]
}

func shortID(value string) string {
	if len(value) <= 14 {
		return value
	}
	return value[:8] + "…" + value[len(value)-4:]
}

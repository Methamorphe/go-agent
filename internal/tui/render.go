package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Methamorphe/go-agent/internal/workspace"
)

const (
	wideLayoutWidth   = 118
	mediumLayoutWidth = 82
	minPaneHeight     = 8
)

type renderStyles struct {
	base       lipgloss.Style
	header     lipgloss.Style
	pane       lipgloss.Style
	paneFocus  lipgloss.Style
	title      lipgloss.Style
	muted      lipgloss.Style
	warning    lipgloss.Style
	danger     lipgloss.Style
	composer   lipgloss.Style
	status     lipgloss.Style
	selected   lipgloss.Style
	overlay    lipgloss.Style
	block      lipgloss.Style
	blockAlert lipgloss.Style
}

func stylesFor(theme Theme) renderStyles {
	color := lipgloss.Color
	base := lipgloss.NewStyle().Foreground(color(theme.Text)).Background(color(theme.Background))
	pane := lipgloss.NewStyle().
		Foreground(color(theme.Text)).
		Background(color(theme.Surface)).
		Border(lipgloss.NormalBorder()).
		BorderForeground(color(theme.Border)).
		Padding(0, 1)
	return renderStyles{
		base:       base,
		header:     lipgloss.NewStyle().Foreground(color(theme.Text)).Background(color(theme.SurfaceAlt)).Bold(true).Padding(0, 1),
		pane:       pane,
		paneFocus:  pane.BorderForeground(color(theme.Focus)),
		title:      lipgloss.NewStyle().Foreground(color(theme.Accent)).Bold(true),
		muted:      lipgloss.NewStyle().Foreground(color(theme.Muted)),
		warning:    lipgloss.NewStyle().Foreground(color(theme.Warning)),
		danger:     lipgloss.NewStyle().Foreground(color(theme.Danger)).Bold(true),
		composer:   lipgloss.NewStyle().Foreground(color(theme.Text)).Background(color(theme.SurfaceAlt)).Border(lipgloss.NormalBorder()).BorderForeground(color(theme.Focus)).Padding(0, 1),
		status:     lipgloss.NewStyle().Foreground(color(theme.Muted)).Background(color(theme.Background)).Padding(0, 1),
		selected:   lipgloss.NewStyle().Foreground(color(theme.Text)).Background(color(theme.SurfaceAlt)).Bold(true),
		overlay:    lipgloss.NewStyle().Foreground(color(theme.Text)).Background(color(theme.Surface)).Border(lipgloss.RoundedBorder()).BorderForeground(color(theme.Focus)).Padding(1, 2),
		block:      lipgloss.NewStyle().Foreground(color(theme.Text)),
		blockAlert: lipgloss.NewStyle().Foreground(color(theme.Danger)).Bold(true),
	}
}

func (m Model) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	view.WindowTitle = "go-agent · Interactive Agent Workspace"
	return view
}

func (m Model) render() string {
	width := m.width
	height := m.height
	if width <= 0 {
		width = 100
	}
	if height <= 0 {
		height = 30
	}

	styles := stylesFor(m.cfg.Theme)
	header := m.renderHeader(styles, width)
	bodyHeight := max(minPaneHeight, height-5)

	var body string
	switch {
	case width >= wideLayoutWidth:
		leftWidth := clamp(width/5, 22, 32)
		rightWidth := clamp(width/4, 28, 40)
		centerWidth := max(32, width-leftWidth-rightWidth-4)
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			m.renderAgents(styles, leftWidth, bodyHeight),
			m.renderConversation(styles, centerWidth, bodyHeight),
			m.renderInspector(styles, rightWidth, bodyHeight),
		)
	case width >= mediumLayoutWidth:
		leftWidth := clamp(width/4, 22, 30)
		centerWidth := max(40, width-leftWidth-2)
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			m.renderAgents(styles, leftWidth, bodyHeight),
			m.renderConversation(styles, centerWidth, bodyHeight),
		)
	default:
		body = m.renderConversation(styles, width, bodyHeight)
	}

	composer := m.renderComposer(styles, width)
	status := m.renderStatus(styles, width)
	page := lipgloss.JoinVertical(lipgloss.Left, header, body, composer, status)
	page = styles.base.Width(width).Height(height).Render(page)
	if m.overlay != overlayNone {
		return m.renderOverlay(styles, page, width, height)
	}
	return page
}

func (m Model) renderHeader(styles renderStyles, width int) string {
	left := fmt.Sprintf(" GO-AGENT  %s  root:%s  agent:%s ", m.mode, shortID(m.rootID.String()), shortID(m.focusID.String()))
	right := fmt.Sprintf(" cursor:%d  cache:%d  ", m.cursor, len(m.blocks))
	available := max(0, width-visibleWidth(left)-visibleWidth(right))
	return styles.header.Width(width).Render(left + strings.Repeat(" ", available) + right)
}

func (m Model) renderAgents(styles renderStyles, width, height int) string {
	innerWidth := max(8, width-4)
	lines := []string{styles.title.Render("AGENTS / TASKS")}
	if len(m.tree) == 0 {
		lines = append(lines, styles.muted.Render("No process projection"))
	}
	start := 0
	visible := max(1, height-4)
	if m.selectedAgent >= visible {
		start = m.selectedAgent - visible + 1
	}
	end := min(len(m.tree), start+visible)
	for index := start; index < end; index++ {
		item := m.tree[index]
		indent := strings.Repeat("  ", int(item.Depth))
		prefix := "  "
		if item.AgentID == m.focusID {
			prefix = "▶ "
		}
		state := statusLabel(string(item.Status))
		goal := oneLine(item.Goal, max(8, innerWidth-visibleWidth(indent)-visibleWidth(prefix)-visibleWidth(state)-2))
		line := fmt.Sprintf("%s%s%s %s", prefix, indent, state, goal)
		if index == m.selectedAgent {
			line = styles.selected.Width(innerWidth).Render(line)
		} else if item.Failure != "" {
			line = styles.danger.Render(line)
		}
		lines = append(lines, line)
	}
	if len(m.tree) > end {
		lines = append(lines, styles.muted.Render(fmt.Sprintf("… %d more", len(m.tree)-end)))
	}
	return styles.pane.Width(width).Height(height).Render(joinAndClip(lines, height-2))
}

func (m Model) renderConversation(styles renderStyles, width, height int) string {
	innerWidth := max(16, width-4)
	lines := []string{styles.title.Render("CONVERSATION / WORK")}
	blocks := m.visibleBlocks(max(1, height-4))
	if len(blocks) == 0 {
		lines = append(lines, styles.muted.Render("No projected conversation blocks yet."))
	}
	for _, block := range blocks {
		label := blockLabel(block.Kind)
		timeLabel := ""
		if !block.OccurredAt.IsZero() {
			timeLabel = block.OccurredAt.Local().Format("15:04:05") + " "
		}
		prefix := timeLabel + label
		preview := block.Preview
		if preview == "" {
			preview = block.Title
		}
		wrapped := wrapText(preview, max(12, innerWidth-visibleWidth(prefix)-1), 2)
		line := prefix + " " + wrapped[0]
		if block.Critical {
			line = styles.blockAlert.Render(line)
		} else {
			line = styles.block.Render(line)
		}
		lines = append(lines, line)
		for _, continuation := range wrapped[1:] {
			lines = append(lines, strings.Repeat(" ", visibleWidth(prefix)+1)+styles.muted.Render(continuation))
		}
		if block.ObjectRef != "" && len(lines) < height-2 {
			lines = append(lines, styles.muted.Render("  ↳ "+oneLine(block.ObjectRef, innerWidth-4)))
		}
	}
	if m.hasPrevious {
		lines = append([]string{styles.muted.Render("↑ PgUp / wheel: older history")}, lines...)
	}
	return styles.paneFocus.Width(width).Height(height).Render(joinAndClip(lines, height-2))
}

func (m Model) visibleBlocks(lineBudget int) []workspace.Block {
	if lineBudget <= 0 || len(m.blocks) == 0 {
		return nil
	}
	end := len(m.blocks) - m.scroll
	end = clamp(end, 0, len(m.blocks))
	blockBudget := max(1, lineBudget/2)
	start := max(0, end-blockBudget)
	return m.blocks[start:end]
}

func (m Model) renderInspector(styles renderStyles, width, height int) string {
	tabs := []string{"runtime", "transactions", "forks", "context", "teams", "improvements"}
	active := int(m.inspectorTab) % len(tabs)
	lines := []string{styles.title.Render("INSPECTOR · " + strings.ToUpper(tabs[active]))}

	switch active {
	case 0:
		lines = append(lines,
			fmt.Sprintf("mode        %s", m.mode),
			fmt.Sprintf("root        %s", shortID(m.rootID.String())),
			fmt.Sprintf("focus       %s", shortID(m.focusID.String())),
			fmt.Sprintf("agents      %d", len(m.tree)),
			fmt.Sprintf("cursor      %d", m.cursor),
			fmt.Sprintf("cache       %d/%d", len(m.blocks), m.cfg.MaxCachedBlocks),
			fmt.Sprintf("refresh     %d", m.refreshCount),
		)
	case 1:
		if len(m.inspector.Transactions) == 0 {
			lines = append(lines, styles.muted.Render("No active/recent transactions"))
		}
		for _, tx := range m.inspector.Transactions {
			line := fmt.Sprintf("%s %s v%d · effects:%d", shortID(tx.ID.String()), tx.State, tx.Version, tx.EffectCount)
			if tx.UncertainEffects > 0 || strings.Contains(tx.State, "RECONCILIATION") {
				lines = append(lines, styles.danger.Render("! "+line))
				lines = append(lines, styles.warning.Render("  outcome uncertainty preserved"))
				if tx.ReconcileReason != "" {
					lines = append(lines, styles.muted.Render("  "+oneLine(tx.ReconcileReason, width-8)))
				}
			} else {
				lines = append(lines, line)
			}
			if tx.VerificationStatus != "" {
				lines = append(lines, styles.muted.Render("  verify: "+tx.VerificationStatus))
			}
		}
	case 2:
		if len(m.inspector.Forks) == 0 {
			lines = append(lines, styles.muted.Render("No cognitive fork groups"))
		}
		for _, fork := range m.inspector.Forks {
			lines = append(lines, fmt.Sprintf("%s %s · branches:%d", shortID(fork.GroupID.String()), fork.State, len(fork.Branches)))
			for _, branch := range fork.Branches {
				marker := "  ├─"
				if branch.ForkID == fork.WinnerForkID {
					marker = "  ★ "
				}
				lines = append(lines, fmt.Sprintf("%s %s %s · %dt", marker, shortID(branch.ForkID.String()), branch.State, branch.SpentTokens))
			}
		}
	case 3:
		if len(m.inspector.ContextFaults) == 0 {
			lines = append(lines, styles.muted.Render("No recent Context Faults"))
		}
		for _, fault := range m.inspector.ContextFaults {
			line := fmt.Sprintf("%s %s · %s", shortID(fault.FaultID.String()), fault.State, fault.Kind)
			if fault.State != "RESOLVED" {
				line = styles.warning.Render("! " + line)
			}
			lines = append(lines, line)
			if fault.Reference != "" {
				lines = append(lines, styles.muted.Render("  "+oneLine(fault.Reference, width-8)))
			}
		}
	case 4:
		if len(m.inspector.Teams) == 0 {
			lines = append(lines, styles.muted.Render("No adaptive teams"))
		}
		for _, team := range m.inspector.Teams {
			line := fmt.Sprintf("%s %s · %d members · round %d/%d", shortID(team.TeamID.String()), team.State, team.MemberCount, team.Round, team.MaxRounds)
			if team.Escalated {
				line = styles.warning.Render("! " + line)
			}
			lines = append(lines, line)
			lines = append(lines, styles.muted.Render("  "+oneLine(team.Objective, width-8)))
		}
	case 5:
		if len(m.inspector.Improvements) == 0 {
			lines = append(lines, styles.muted.Render("No improvement artifacts"))
		}
		for _, item := range m.inspector.Improvements {
			flags := ""
			if item.Active {
				flags += " active"
			}
			if item.Killed {
				flags += " KILLED"
			}
			line := fmt.Sprintf("%s v%d %s%s", oneLine(item.ArtifactID, 14), item.Version, item.Status, flags)
			if item.Killed {
				line = styles.danger.Render(line)
			}
			lines = append(lines, line)
			lines = append(lines, styles.muted.Render("  "+item.Kind+" · "+item.ScopeLevel))
		}
	}
	lines = append(lines, styles.muted.Render("[ / ] switch inspector"))
	return styles.pane.Width(width).Height(height).Render(joinAndClip(lines, height-2))
}

func (m Model) renderComposer(styles renderStyles, width int) string {
	queue := "STEER"
	if !m.composerFocused {
		queue = "press i to compose"
	}
	text := m.composer
	if text == "" {
		text = "message the focused Agent"
	}
	line := fmt.Sprintf("%s › %s", queue, text)
	return styles.composer.Width(width).Render(oneLine(line, max(8, width-4)))
}

func (m Model) renderStatus(styles renderStyles, width int) string {
	text := m.status
	if m.err != nil {
		text = "ERROR: " + m.err.Error()
	}
	if text == "" {
		text = "Ctrl+P palette · ? help · 1–5 modes · PgUp history · [/] inspector · q detach"
	}
	if m.loading {
		text = "working · " + text
	}
	if m.err != nil {
		return styles.danger.Width(width).Render(oneLine(text, width))
	}
	return styles.status.Width(width).Render(oneLine(text, width))
}

func (m Model) renderOverlay(styles renderStyles, page string, width, height int) string {
	var lines []string
	switch m.overlay {
	case overlayPalette:
		lines = []string{
			styles.title.Render("COMMAND PALETTE"),
			"attach / refresh         refresh focused workspace",
			"mode ask|plan|act|review|observe",
			"agent next|prev          focus another Agent",
			"suspend / resume         canonical process controls",
			"search <query>           durable projected history",
			"steer <message>          active-work guidance",
			"follow <message>         queued continuation",
			"plan <feedback>          plan-review intent",
			"review <comment>         diff/review feedback",
			"theme dark|light         client-local presentation",
			"detach                   leave runtime work running",
			"",
			"> " + m.overlayInput,
		}
	case overlaySearch:
		lines = []string{styles.title.Render("HISTORY SEARCH"), "> " + m.overlayInput}
		for _, block := range firstBlocks(m.searchResults, 12) {
			lines = append(lines, fmt.Sprintf("%s %s", shortID(block.ID), oneLine(block.Preview, 72)))
		}
	case overlayHelp:
		lines = []string{
			styles.title.Render("KEYBOARD WORKFLOW"),
			"i / Enter      focus composer",
			"Enter          steer at next safe boundary",
			"Alt+Enter      queue follow-up",
			"Ctrl+P         command palette",
			"/              history search",
			"↑ ↓ / j k      select and attach Agent",
			"PgUp / wheel   load/scroll history",
			"[ / ]          switch inspector",
			"1..5           ASK / PLAN / ACT / REVIEW / OBSERVE",
			"s              suspend/resume focused Agent",
			"q / Ctrl+C     detach TUI only; Agent continues",
			"Esc            close overlay/editor",
		}
	default:
		return page
	}
	boxWidth := clamp(width-8, 34, 88)
	box := styles.overlay.Width(boxWidth).Render(joinAndClip(lines, max(6, height-8)))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func firstBlocks(blocks []workspace.Block, limit int) []workspace.Block {
	if len(blocks) <= limit {
		return blocks
	}
	return blocks[:limit]
}

func blockLabel(kind workspace.BlockKind) string {
	switch kind {
	case workspace.BlockUserMessage:
		return "USER"
	case workspace.BlockAssistantMessage:
		return "AGENT"
	case workspace.BlockPlan:
		return "PLAN"
	case workspace.BlockToolCall:
		return "TOOL→"
	case workspace.BlockToolResult:
		return "TOOL✓"
	case workspace.BlockDiff:
		return "DIFF"
	case workspace.BlockTestResult:
		return "TEST"
	case workspace.BlockApproval:
		return "APPROVAL"
	case workspace.BlockTransaction:
		return "TX"
	case workspace.BlockForkComparison:
		return "FORK"
	case workspace.BlockContextFault:
		return "CTX!"
	case workspace.BlockSecurityDecision:
		return "AUTH"
	case workspace.BlockCheckpoint:
		return "CHECKPOINT"
	case workspace.BlockError:
		return "ERROR"
	default:
		return "·"
	}
}

func statusLabel(status string) string {
	switch status {
	case "RUNNING":
		return "RUN"
	case "READY":
		return "RDY"
	case "WAITING":
		return "WAIT"
	case "SLEEPING":
		return "SLP"
	case "SUSPENDED":
		return "PAUSE"
	case "COMPLETED":
		return "DONE"
	case "FAILED":
		return "FAIL"
	case "CANCELLED":
		return "CANCEL"
	default:
		return oneLine(status, 6)
	}
}

func wrapText(value string, width, maxLines int) []string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return []string{""}
	}
	width = max(width, 4)
	words := strings.Fields(value)
	lines := make([]string, 0, maxLines)
	var current strings.Builder
	for _, word := range words {
		if current.Len() == 0 {
			current.WriteString(oneLine(word, width))
			continue
		}
		if visibleWidth(current.String())+1+visibleWidth(word) <= width {
			current.WriteByte(' ')
			current.WriteString(word)
			continue
		}
		lines = append(lines, current.String())
		if len(lines) >= maxLines {
			lines[len(lines)-1] = oneLine(lines[len(lines)-1], max(1, width-1)) + "…"
			return lines
		}
		current.Reset()
		current.WriteString(oneLine(word, width))
	}
	if current.Len() > 0 && len(lines) < maxLines {
		lines = append(lines, current.String())
	}
	return lines
}

func oneLine(value string, width int) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.Join(strings.Fields(value), " ")
	if width <= 0 {
		return ""
	}
	if utf8.RuneCountInString(value) <= width {
		return value
	}
	runes := []rune(value)
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}

func visibleWidth(value string) int { return lipgloss.Width(value) }

func joinAndClip(lines []string, height int) string {
	if height <= 0 {
		return ""
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

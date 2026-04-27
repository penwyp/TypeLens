package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/penwyp/typelens/internal/service"
	"github.com/penwyp/typelens/pkg/typeless"
	"github.com/spf13/cobra"
)

func newAutoImportCommand(svc *service.Service) *cobra.Command {
	return &cobra.Command{
		Use:   "auto-import",
		Short: "以交互式 TUI 方式扫描并导入候选词",
		RunE: func(cmd *cobra.Command, args []string) error {
			model := newAutoImportModel(cmd.Context(), svc)
			program := tea.NewProgram(model)
			finalModel, err := program.Run()
			if err != nil {
				return err
			}
			resultModel, ok := finalModel.(autoImportModel)
			if !ok {
				return nil
			}
			if resultModel.finalErr != nil {
				return resultModel.finalErr
			}
			if resultModel.cancelled {
				return nil
			}
			if resultModel.importResultData != nil {
				fmt.Printf("已处理 %d 个候选词，当前剩余待同步 %d 个。\n", resultModel.importResultData.AcceptedCount, len(resultModel.importResultData.Words))
			}
			return nil
		},
	}
}

type autoImportPhase string

const (
	autoImportPhaseLoading    autoImportPhase = "loading"
	autoImportPhaseSources    autoImportPhase = "sources"
	autoImportPhaseEditing    autoImportPhase = "editing"
	autoImportPhaseScanning   autoImportPhase = "scanning"
	autoImportPhaseCandidates autoImportPhase = "candidates"
	autoImportPhaseSyncing    autoImportPhase = "syncing"
	autoImportPhaseDone       autoImportPhase = "done"
)

type autoImportModel struct {
	ctx context.Context
	svc *service.Service

	phase autoImportPhase

	sources []typeless.AutoImportSource
	cursor  int

	input        textinput.Model
	editingIndex int
	editingNew   bool

	logs []string

	candidates  []typeless.AutoImportCandidate
	selected    map[string]bool
	scanSummary *typeless.AutoImportScanResult

	logCh        <-chan string
	scanResult   <-chan scanResultMsg
	importResult <-chan importResultMsg

	importedCount    int
	cancelled        bool
	finalErr         error
	statusText       string
	importResultData *service.AutoImportConfirmResult
}

type defaultsLoadedMsg struct {
	sources []typeless.AutoImportSource
	err     error
}

type logLineMsg string
type logStreamClosedMsg struct{}

type scanResultMsg struct {
	result typeless.AutoImportScanResult
	err    error
}

type importResultMsg struct {
	result service.AutoImportConfirmResult
	err    error
}

func newAutoImportModel(ctx context.Context, svc *service.Service) autoImportModel {
	input := textinput.New()
	input.Prompt = "目录: "
	input.CharLimit = 0
	input.Width = 72
	return autoImportModel{
		ctx:          ctx,
		svc:          svc,
		phase:        autoImportPhaseLoading,
		selected:     make(map[string]bool),
		input:        input,
		editingIndex: -1,
	}
}

func (m autoImportModel) Init() tea.Cmd {
	return loadDefaultSourcesCmd(m.svc)
}

func (m autoImportModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.updateKey(msg)
	case defaultsLoadedMsg:
		if msg.err != nil {
			m.finalErr = msg.err
			return m, tea.Quit
		}
		m.sources = msg.sources
		m.phase = autoImportPhaseSources
		return m, nil
	case logLineMsg:
		m.logs = appendLimited(m.logs, string(msg))
		return m, waitLogLineCmd(m.logCh)
	case logStreamClosedMsg:
		return m, nil
	case scanResultMsg:
		if msg.err != nil {
			m.statusText = msg.err.Error()
			m.phase = autoImportPhaseSources
			return m, nil
		}
		m.statusText = ""
		m.scanSummary = &msg.result
		m.candidates = msg.result.Items
		m.selected = make(map[string]bool, len(msg.result.Items))
		for _, item := range msg.result.Items {
			m.selected[item.NormalizedTerm] = true
		}
		m.cursor = 0
		m.phase = autoImportPhaseCandidates
		return m, nil
	case importResultMsg:
		if msg.err != nil {
			m.statusText = msg.err.Error()
			m.phase = autoImportPhaseCandidates
			return m, nil
		}
		m.statusText = ""
		m.importResultData = &msg.result
		m.importedCount = msg.result.AcceptedCount
		m.phase = autoImportPhaseDone
		return m, nil
	}

	if m.phase == autoImportPhaseEditing {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m autoImportModel) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.phase {
	case autoImportPhaseLoading, autoImportPhaseScanning, autoImportPhaseSyncing:
		if keyMatches(msg, "ctrl+c", "q") {
			m.cancelled = true
			return m, tea.Quit
		}
		return m, nil
	case autoImportPhaseEditing:
		switch {
		case keyMatches(msg, "esc"):
			if m.editingNew && m.editingIndex >= 0 && m.editingIndex < len(m.sources) && strings.TrimSpace(m.sources[m.editingIndex].Workdir) == "" {
				m.sources = append(m.sources[:m.editingIndex], m.sources[m.editingIndex+1:]...)
				if m.cursor >= len(m.sources) {
					m.cursor = max(0, len(m.sources)-1)
				}
			}
			m.editingIndex = -1
			m.editingNew = false
			m.phase = autoImportPhaseSources
			return m, nil
		case keyMatches(msg, "enter"):
			if m.editingIndex < 0 || m.editingIndex >= len(m.sources) {
				m.phase = autoImportPhaseSources
				return m, nil
			}
			m.sources[m.editingIndex].Workdir = strings.TrimSpace(m.input.Value())
			m.editingIndex = -1
			m.editingNew = false
			m.phase = autoImportPhaseSources
			return m, nil
		}
	case autoImportPhaseSources:
		switch {
		case keyMatches(msg, "ctrl+c", "q"):
			m.cancelled = true
			return m, tea.Quit
		case keyMatches(msg, "up", "k"):
			m.cursor = moveCursor(m.cursor, len(m.sources), -1)
		case keyMatches(msg, "down", "j"):
			m.cursor = moveCursor(m.cursor, len(m.sources), 1)
		case keyMatches(msg, " "):
			if len(m.sources) > 0 {
				m.sources[m.cursor].Enabled = !m.sources[m.cursor].Enabled
			}
		case keyMatches(msg, "e"):
			if len(m.sources) == 0 {
				return m, nil
			}
			m.editingIndex = m.cursor
			m.editingNew = false
			m.input.SetValue(m.sources[m.cursor].Workdir)
			m.input.CursorEnd()
			m.input.Focus()
			m.phase = autoImportPhaseEditing
			return m, textinput.Blink
		case keyMatches(msg, "a"):
			m.sources = append(m.sources, typeless.AutoImportSource{
				Platform: typeless.AutoImportPlatformCustom,
				Enabled:  true,
				Workdir:  "",
			})
			m.cursor = len(m.sources) - 1
			m.editingIndex = m.cursor
			m.editingNew = true
			m.input.SetValue("")
			m.input.Focus()
			m.phase = autoImportPhaseEditing
			return m, textinput.Blink
		case keyMatches(msg, "d", "backspace"):
			if len(m.sources) == 0 {
				return m, nil
			}
			if m.sources[m.cursor].Platform == typeless.AutoImportPlatformCustom {
				m.sources = append(m.sources[:m.cursor], m.sources[m.cursor+1:]...)
				if m.cursor >= len(m.sources) {
					m.cursor = max(0, len(m.sources)-1)
				}
			}
		case keyMatches(msg, "enter", "s"):
			if !hasEnabledSources(m.sources) {
				return m, nil
			}
			logCh := make(chan string, 256)
			resultCh := make(chan scanResultMsg, 1)
			m.logs = nil
			m.logCh = logCh
			m.scanResult = resultCh
			m.phase = autoImportPhaseScanning
			return m, tea.Batch(
				startScanCmd(m.ctx, m.svc, slicesCloneSources(m.sources), logCh, resultCh),
				waitLogLineCmd(logCh),
				waitScanResultCmd(resultCh),
			)
		}
		return m, nil
	case autoImportPhaseCandidates:
		switch {
		case keyMatches(msg, "ctrl+c", "q"):
			m.cancelled = true
			return m, tea.Quit
		case keyMatches(msg, "esc"):
			m.phase = autoImportPhaseSources
		case keyMatches(msg, "up", "k"):
			m.cursor = moveCursor(m.cursor, len(m.candidates), -1)
		case keyMatches(msg, "down", "j"):
			m.cursor = moveCursor(m.cursor, len(m.candidates), 1)
		case keyMatches(msg, " "):
			if len(m.candidates) > 0 {
				key := m.candidates[m.cursor].NormalizedTerm
				m.selected[key] = !m.selected[key]
			}
		case keyMatches(msg, "A"):
			for _, item := range m.candidates {
				m.selected[item.NormalizedTerm] = true
			}
		case keyMatches(msg, "n"):
			for _, item := range m.candidates {
				m.selected[item.NormalizedTerm] = false
			}
		case keyMatches(msg, "enter"):
			items := m.selectedCandidates()
			if len(items) == 0 {
				return m, nil
			}
			logCh := make(chan string, 256)
			resultCh := make(chan importResultMsg, 1)
			m.logs = nil
			m.logCh = logCh
			m.importResult = resultCh
			m.phase = autoImportPhaseSyncing
			return m, tea.Batch(
				startImportCmd(m.ctx, m.svc, items, logCh, resultCh),
				waitLogLineCmd(logCh),
				waitImportResultCmd(resultCh),
			)
		}
		return m, nil
	case autoImportPhaseDone:
		if keyMatches(msg, "enter", "q", "esc", "ctrl+c") {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m autoImportModel) View() string {
	switch m.phase {
	case autoImportPhaseLoading:
		return "Loading auto-import sources..."
	case autoImportPhaseSources:
		return m.viewSources()
	case autoImportPhaseEditing:
		return m.viewEditing()
	case autoImportPhaseScanning:
		return m.viewLogs("Scanning candidate terms")
	case autoImportPhaseCandidates:
		return m.viewCandidates()
	case autoImportPhaseSyncing:
		return m.viewLogs("Syncing selected terms")
	case autoImportPhaseDone:
		return m.viewDone()
	default:
		return ""
	}
}

func (m autoImportModel) viewSources() string {
	var builder strings.Builder
	builder.WriteString("TypeLens Auto Import\n\n")
	builder.WriteString("Sources\n")
	builder.WriteString("  up/down: move  space: toggle  e: edit dir  a: add other dir  d: delete custom dir  enter: scan  q: quit\n\n")
	for index, source := range m.sources {
		cursor := " "
		if index == m.cursor {
			cursor = ">"
		}
		check := "[ ]"
		if source.Enabled {
			check = "[x]"
		}
		builder.WriteString(fmt.Sprintf("%s %s %-10s %s\n", cursor, check, sourceName(source.Platform), source.Workdir))
	}
	if len(m.sources) == 0 {
		builder.WriteString("  No source configured.\n")
	}
	if m.statusText != "" {
		builder.WriteString("\nError: ")
		builder.WriteString(m.statusText)
		builder.WriteString("\n")
	}
	builder.WriteString("\nHint: source label is only a local marker. Import is text-driven.\n")
	return builder.String()
}

func (m autoImportModel) viewEditing() string {
	var builder strings.Builder
	builder.WriteString("TypeLens Auto Import\n\n")
	builder.WriteString("Edit Directory\n")
	builder.WriteString("  enter: save  esc: cancel\n\n")
	builder.WriteString(m.input.View())
	builder.WriteString("\n")
	return builder.String()
}

func (m autoImportModel) viewLogs(title string) string {
	var builder strings.Builder
	builder.WriteString("TypeLens Auto Import\n\n")
	builder.WriteString(title)
	builder.WriteString("\n\n")
	for _, line := range m.logs {
		builder.WriteString("- ")
		builder.WriteString(line)
		builder.WriteString("\n")
	}
	if len(m.logs) == 0 {
		builder.WriteString("Waiting for logs...\n")
	}
	builder.WriteString("\nPress q to quit.\n")
	return builder.String()
}

func (m autoImportModel) viewCandidates() string {
	var builder strings.Builder
	builder.WriteString("TypeLens Auto Import\n\n")
	if m.scanSummary != nil {
		builder.WriteString(fmt.Sprintf(
			"Scan summary: files=%d messages=%d raw=%d filtered=%d\n\n",
			m.scanSummary.ScannedFiles,
			m.scanSummary.ParsedMessages,
			m.scanSummary.RawCandidates,
			m.scanSummary.FilteredCandidates,
		))
	}
	builder.WriteString("Candidates\n")
	builder.WriteString("  up/down: move  space: toggle  A: select all  n: clear all  enter: import selected  esc: back\n\n")
	for index, item := range m.candidates {
		cursor := " "
		if index == m.cursor {
			cursor = ">"
		}
		check := "[ ]"
		if m.selected[item.NormalizedTerm] {
			check = "[x]"
		}
		builder.WriteString(fmt.Sprintf("%s %s %-24s %s · %d hits\n", cursor, check, item.Term, sourceName(item.Platform), item.Hits))
		if len(item.Examples) > 0 {
			builder.WriteString(fmt.Sprintf("      %s\n", item.Examples[0]))
		}
	}
	if len(m.candidates) == 0 {
		builder.WriteString("  No candidate terms found.\n")
	}
	if m.statusText != "" {
		builder.WriteString("\nError: ")
		builder.WriteString(m.statusText)
		builder.WriteString("\n")
	}
	builder.WriteString(fmt.Sprintf("\nSelected: %d/%d\n", len(m.selectedCandidates()), len(m.candidates)))
	return builder.String()
}

func (m autoImportModel) viewDone() string {
	var builder strings.Builder
	builder.WriteString("TypeLens Auto Import\n\n")
	builder.WriteString(fmt.Sprintf("Import finished. Accepted %d term(s).\n\n", m.importedCount))
	for _, line := range m.logs {
		builder.WriteString("- ")
		builder.WriteString(line)
		builder.WriteString("\n")
	}
	builder.WriteString("\nPress enter or q to exit.\n")
	return builder.String()
}

func (m autoImportModel) selectedCandidates() []typeless.AutoImportCandidate {
	items := make([]typeless.AutoImportCandidate, 0, len(m.candidates))
	for _, item := range m.candidates {
		if m.selected[item.NormalizedTerm] {
			items = append(items, item)
		}
	}
	return items
}

func loadDefaultSourcesCmd(_ *service.Service) tea.Cmd {
	return func() tea.Msg {
		sources, err := typeless.DefaultAutoImportSources()
		if err != nil {
			return defaultsLoadedMsg{err: err}
		}
		return defaultsLoadedMsg{sources: sources}
	}
}

func startScanCmd(
	ctx context.Context,
	svc *service.Service,
	sources []typeless.AutoImportSource,
	logCh chan<- string,
	resultCh chan<- scanResultMsg,
) tea.Cmd {
	return func() tea.Msg {
		go func() {
			defer close(logCh)
			defer close(resultCh)
			result, err := svc.ScanAutoImport(ctx, service.AutoImportScanRequest{
				Sources: sources,
			}, newLineChannelWriter(logCh))
			resultCh <- scanResultMsg{
				result: result,
				err:    err,
			}
		}()
		return nil
	}
}

func startImportCmd(
	ctx context.Context,
	svc *service.Service,
	items []typeless.AutoImportCandidate,
	logCh chan<- string,
	resultCh chan<- importResultMsg,
) tea.Cmd {
	return func() tea.Msg {
		go func() {
			defer close(logCh)
			defer close(resultCh)
			result, err := svc.ConfirmAutoImportSync(ctx, service.AutoImportConfirmRequest{
				Items: items,
			}, newLineChannelWriter(logCh))
			resultCh <- importResultMsg{
				result: result,
				err:    err,
			}
		}()
		return nil
	}
}

func waitLogLineCmd(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			return nil
		}
		line, ok := <-ch
		if !ok {
			return logStreamClosedMsg{}
		}
		return logLineMsg(line)
	}
}

func waitScanResultCmd(ch <-chan scanResultMsg) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			return nil
		}
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func waitImportResultCmd(ch <-chan importResultMsg) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			return nil
		}
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

type lineChannelWriter struct {
	ch chan<- string
}

func newLineChannelWriter(ch chan<- string) io.Writer {
	return &lineChannelWriter{ch: ch}
}

func (w *lineChannelWriter) Write(p []byte) (int, error) {
	text := strings.TrimSpace(string(p))
	if text == "" {
		return len(p), nil
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		w.ch <- line
	}
	return len(p), nil
}

func keyMatches(msg tea.KeyMsg, keys ...string) bool {
	for _, key := range keys {
		if msg.String() == key {
			return true
		}
	}
	return false
}

func moveCursor(cursor, length, delta int) int {
	if length == 0 {
		return 0
	}
	next := cursor + delta
	if next < 0 {
		return 0
	}
	if next >= length {
		return length - 1
	}
	return next
}

func appendLimited(lines []string, line string) []string {
	lines = append(lines, line)
	if len(lines) <= 200 {
		return lines
	}
	return lines[len(lines)-200:]
}

func hasEnabledSources(sources []typeless.AutoImportSource) bool {
	for _, source := range sources {
		if source.Enabled && strings.TrimSpace(source.Workdir) != "" {
			return true
		}
	}
	return false
}

func sourceName(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case typeless.AutoImportPlatformCodex:
		return "Codex"
	case typeless.AutoImportPlatformClaude:
		return "Claude"
	default:
		return "Other"
	}
}

func slicesCloneSources(sources []typeless.AutoImportSource) []typeless.AutoImportSource {
	cloned := make([]typeless.AutoImportSource, len(sources))
	copy(cloned, sources)
	return cloned
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}

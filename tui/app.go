package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/mattn/go-runewidth"

	"github.com/cpra/muon/agent"
	"github.com/cpra/muon/config"
	"github.com/cpra/muon/llm"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// entryKind distinguishes conversation entry types for styling.
type entryKind int

const (
	entryUser entryKind = iota
	entryAssistant
	entryTool
	entryError
)

// entry represents one visual block in the conversation area.
type entry struct {
	kind     entryKind
	content  string
	detail   string // for tool entries: result preview
	markdown markdownDocument
}

type appEventKind int

const (
	appEventAgent appEventKind = iota
	appEventDone
)

type appEvent struct {
	kind       appEventKind
	agentEvent agent.Event
	content    string
	err        error
	session    *agent.Session
}

type styledLine struct {
	spans     []styledSpan
	fillStyle tcell.Style
	fill      bool
}

type styledSpan struct {
	text  string
	style tcell.Style
}

// App is the top-level tcell application for the muon TUI.
type App struct {
	agent      *agent.Agent
	client     *llm.Client
	config     *config.Config
	workingDir string

	entries []entry
	session *agent.Session
	working bool

	spinnerFrame int

	cost       llm.CostInfo
	usage      llm.Usage
	turnUsage  llm.Usage
	contextLen int

	width     int
	height    int
	scrollTop int

	input  []rune
	cursor int

	eventCh chan appEvent

	screenMu sync.RWMutex
	screen   tcell.Screen
}

// New creates a new TUI app.
func New(client *llm.Client, cfg *config.Config, workingDir string) *App {
	return &App{
		client:     client,
		config:     cfg,
		workingDir: workingDir,
		contextLen: client.ContextLength(),
		eventCh:    make(chan appEvent, 256),
	}
}

// SetAgent attaches the agent used for prompt execution.
func (a *App) SetAgent(agent *agent.Agent) {
	a.agent = agent
}

// Hook receives agent events and wakes the UI loop.
func (a *App) Hook(e agent.Event) {
	select {
	case a.eventCh <- appEvent{kind: appEventAgent, agentEvent: e}:
		a.wake()
	default:
	}
}

// Run starts the TUI loop.
func (a *App) Run() error {
	if a.agent == nil {
		return fmt.Errorf("tui agent is not configured")
	}

	s, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err := s.Init(); err != nil {
		return err
	}
	defer s.Fini()
	s.SetStyle(baseStyle)

	a.screenMu.Lock()
	a.screen = s
	a.screenMu.Unlock()
	defer func() {
		a.screenMu.Lock()
		a.screen = nil
		a.screenMu.Unlock()
	}()

	a.width, a.height = s.Size()
	a.scrollToBottom()

	for {
		a.drainEvents()
		a.width, a.height = s.Size()
		a.draw(s)

		ev := a.waitForEvent(s)
		switch ev := ev.(type) {
		case *tcell.EventResize:
			s.Sync()
		case *tcell.EventInterrupt:
			continue
		case *tcell.EventKey:
			if a.handleKey(ev, s) {
				return nil
			}
		}
	}
}

// Session returns the current agent session, or nil if no conversation has
// started yet.
func (a *App) Session() *agent.Session {
	return a.session
}

func (a *App) waitForEvent(s tcell.Screen) tcell.Event {
	if a.working {
		select {
		case ev := <-s.EventQ():
			return ev
		case <-time.After(80 * time.Millisecond):
			a.spinnerFrame = (a.spinnerFrame + 1) % len(spinnerFrames)
			return tcell.NewEventInterrupt(nil)
		}
	}
	return <-s.EventQ()
}

// --- layout helpers ---

const (
	footerHeight  = 1
	textareaGap   = 1
	maxInputLines = 10
)

func (a *App) layoutMetrics() (bodyHeight, inputY, inputHeight, footerY int) {
	innerWidth := max(a.width-4, 1)
	inputLines, _, _, _ := a.layoutInput(innerWidth)
	inputHeight = len(inputLines) + 2
	footerY = a.height - footerHeight
	inputY = footerY - inputHeight
	bodyHeight = inputY - textareaGap
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	return bodyHeight, inputY, inputHeight, footerY
}

// --- prompt submission ---

func (a *App) submitPrompt() {
	prompt := string(a.input)
	a.input = nil
	a.cursor = 0
	a.entries = append(a.entries, entry{kind: entryUser, content: prompt})
	a.working = true
	a.spinnerFrame = 0
	a.turnUsage = llm.Usage{}
	a.scrollToBottom()
	go a.runAgent(prompt)
}

func (a *App) runAgent(prompt string) {
	sess := a.session
	var (
		content string
		err     error
		newSess *agent.Session
	)

	if sess == nil {
		newSess, content, err = a.agent.Start(context.Background(), prompt)
	} else {
		content, err = sess.Continue(context.Background(), prompt)
	}

	a.eventCh <- appEvent{kind: appEventDone, content: content, err: err, session: newSess}
	a.wake()
}

func (a *App) drainEvents() {
	for {
		select {
		case ev := <-a.eventCh:
			a.handleAppEvent(ev)
		default:
			return
		}
	}
}

func (a *App) handleAppEvent(ev appEvent) {
	switch ev.kind {
	case appEventAgent:
		a.processEvent(ev.agentEvent)
		a.scrollToBottom()
	case appEventDone:
		a.working = false
		if ev.session != nil {
			a.session = ev.session
		}
		if ev.err != nil {
			a.entries = append(a.entries, entry{kind: entryError, content: ev.err.Error()})
		} else if ev.content != "" {
			a.entries = append(a.entries, entry{
				kind:     entryAssistant,
				content:  ev.content,
				markdown: parseMarkdownDocument(ev.content),
			})
		}
		a.scrollToBottom()
	}
}

func (a *App) wake() {
	a.screenMu.RLock()
	s := a.screen
	a.screenMu.RUnlock()
	if s != nil {
		select {
		case s.EventQ() <- tcell.NewEventInterrupt(nil):
		default:
		}
	}
}

// --- event processing ---

func (a *App) processEvent(e agent.Event) {
	switch ev := e.(type) {
	case agent.ToolCallEvent:
		argsParts := make([]string, 0, len(ev.Args))
		keys := make([]string, 0, len(ev.Args))
		for k := range ev.Args {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := ev.Args[k]
			argsParts = append(argsParts, fmt.Sprintf("%s=%v", k, v))
		}
		resultPreview := ""
		if ev.Name != "read" {
			resultPreview = ev.Result.Content
			if len(resultPreview) > 200 {
				resultPreview = resultPreview[:200] + "…"
			}
		}
		a.entries = append(a.entries, entry{
			kind:    entryTool,
			content: fmt.Sprintf("%s %s", ev.Name, strings.Join(argsParts, " ")),
			detail:  resultPreview,
		})
	case agent.TurnEndEvent:
		a.usage = ev.AccumulatedUsage
		a.cost = ev.AccumulatedCost
	case agent.LLMResponseEvent:
		a.turnUsage = ev.Usage
	}
}

// --- rendering ---

func (a *App) draw(s tcell.Screen) {
	s.Clear()
	if a.width < 12 || a.height < 6 {
		drawText(s, 0, 0, a.width, "Terminal too small", errorStyle)
		s.Show()
		return
	}

	bodyHeight, inputY, _, footerY := a.layoutMetrics()
	lines := a.renderConversationLines(a.width)
	maxTop := max(len(lines)-bodyHeight, 0)
	if a.scrollTop > maxTop {
		a.scrollTop = maxTop
	}
	if a.scrollTop < 0 {
		a.scrollTop = 0
	}

	for y := 0; y < bodyHeight && a.scrollTop+y < len(lines); y++ {
		line := lines[a.scrollTop+y]
		drawStyledLine(s, 0, y, a.width, line)
	}

	a.drawInputBox(s, inputY)
	a.drawFooter(s, footerY)
	s.Show()
}

func (a *App) renderConversationLines(width int) []styledLine {
	if width < 1 {
		width = 1
	}

	lines := make([]styledLine, 0, len(a.entries)*4+16)
	for _, line := range LogoLines(width) {
		lines = append(lines, styledLine{spans: []styledSpan{{text: line, style: logoStyle}}})
	}
	lines = append(lines, styledLine{}, styledLine{})

	for _, e := range a.entries {
		switch e.kind {
		case entryUser:
			lines = appendWrapped(lines, wrapPrefixedText("❯ ", "  ", e.content, width), userStyle)
			lines = append(lines, styledLine{}, styledLine{})
		case entryAssistant:
			lines = append(lines, renderMarkdownDocument(e.markdown, width)...)
			lines = append(lines, styledLine{}, styledLine{})
		case entryTool:
			lines = appendWrapped(lines, wrapPrefixedText("  🔧 ", "    ", e.content, width), toolStyle)
			if e.detail != "" {
				lines = appendWrapped(lines, wrapPrefixedText("    ", "    ", e.detail, width), toolResultStyle)
			}
			lines = append(lines, styledLine{})
		case entryError:
			lines = appendWrapped(lines, wrapPrefixedText("✗ ", "  ", e.content, width), errorStyle)
			lines = append(lines, styledLine{}, styledLine{})
		}
	}

	if a.working {
		frame := spinnerFrames[a.spinnerFrame%len(spinnerFrames)]
		var spans []styledSpan
		spans = append(spans, styledSpan{text: "  " + frame + " ", style: spinnerStyle})
		spans = append(spans, styledSpan{text: "thinking", style: dimStyle})
		if a.turnUsage.PromptTokens > 0 || a.turnUsage.CompletionTokens > 0 {
			tokenText := fmt.Sprintf(" · ↑%s ↓%s", formatTokens(a.turnUsage.PromptTokens), formatTokens(a.turnUsage.CompletionTokens))
			spans = append(spans, styledSpan{text: tokenText, style: dimStyle})
		}
		lines = append(lines, styledLine{spans: spans})
	}

	return lines
}

func appendWrapped(lines []styledLine, wrapped []string, style tcell.Style) []styledLine {
	for _, line := range wrapped {
		if line == "" {
			lines = append(lines, styledLine{})
			continue
		}
		lines = append(lines, styledLine{spans: []styledSpan{{text: line, style: style}}})
	}
	return lines
}

func (a *App) drawInputBox(s tcell.Screen, y int) {
	if y < 0 || a.width < 4 {
		s.HideCursor()
		return
	}

	boxX := 1
	boxW := a.width - 2
	if boxW < 3 {
		s.HideCursor()
		return
	}
	innerWidth := max(boxW-2, 1)
	lines, cursorX, cursorY, _ := a.layoutInput(innerWidth)
	boxH := len(lines) + 2
	if y+boxH > a.height-1 {
		boxH = max(a.height-1-y, 0)
	}
	if boxH < 3 {
		s.HideCursor()
		return
	}

	drawBox(s, boxX, y, boxW, boxH, inputBorderStyle)
	for i, line := range lines {
		lineY := y + 1 + i
		if lineY >= y+boxH-1 {
			break
		}
		fillLine(s, boxX+1, lineY, innerWidth, inputTextStyle)
		drawStyledInputLine(s, boxX+1, lineY, innerWidth, line)
	}

	if a.working {
		s.HideCursor()
		return
	}

	showX := boxX + 1 + min(cursorX, innerWidth-1)
	showY := y + 1 + cursorY
	if showY >= y+boxH-1 {
		showY = y + boxH - 2
	}
	if showX < boxX+1 {
		showX = boxX + 1
	}
	s.ShowCursor(showX, showY)
}

func (a *App) layoutInput(width int) ([]string, int, int, int) {
	if width < 1 {
		width = 1
	}

	const firstPrefix = "❯ "
	const restPrefix = "  "
	firstPrefixWidth := runewidth.StringWidth(firstPrefix)
	restPrefixWidth := runewidth.StringWidth(restPrefix)

	lines := []string{firstPrefix}
	lineWidths := []int{firstPrefixWidth}
	linePrefixWidths := []int{firstPrefixWidth}
	currentLine := 0
	cursorX, cursorY := firstPrefixWidth, 0

	for i, r := range a.input {
		if i == a.cursor {
			cursorX = lineWidths[currentLine]
			cursorY = currentLine
		}

		rw := runeWidth(r)
		if lineWidths[currentLine]+rw > width && lineWidths[currentLine] > linePrefixWidths[currentLine] {
			lines = append(lines, restPrefix)
			lineWidths = append(lineWidths, restPrefixWidth)
			linePrefixWidths = append(linePrefixWidths, restPrefixWidth)
			currentLine++
		}

		lines[currentLine] += string(r)
		lineWidths[currentLine] += rw
	}

	if a.cursor == len(a.input) {
		cursorX = lineWidths[currentLine]
		cursorY = currentLine
	}

	totalLines := len(lines)
	start := 0
	if totalLines > maxInputLines {
		start = cursorY - maxInputLines + 1
		if start < 0 {
			start = 0
		}
		if start+maxInputLines > totalLines {
			start = totalLines - maxInputLines
		}
		lines = lines[start : start+maxInputLines]
		cursorY -= start
	}

	return lines, cursorX, cursorY, totalLines
}

func (a *App) drawFooter(s tcell.Screen, y int) {
	if y < 0 || y >= a.height {
		return
	}
	fillLine(s, 0, y, a.width, footerStyle)

	left := a.workingDir
	drawText(s, 0, y, a.width, left, footerDimStyle)

	provider := footerSegment{text: a.config.ProviderName + "/" + a.config.Model, style: footerDimStyle}
	cost := a.renderCost(a.cost.TotalCost)
	ctxUsed := a.usage.PromptTokens + a.usage.CompletionTokens
	ctx := a.renderContextBar(ctxUsed, a.contextLen)
	sep := footerSegment{text: " │ ", style: footerDimStyle}
	right := []footerSegment{provider, sep, cost, sep, ctx}
	rightWidth := 0
	for _, seg := range right {
		rightWidth += runewidth.StringWidth(seg.text)
	}
	x := a.width - rightWidth
	if x < 0 {
		x = 0
	}
	for _, seg := range right {
		x += drawText(s, x, y, a.width-x, seg.text, seg.style)
	}
}

type footerSegment struct {
	text  string
	style tcell.Style
}

func (a *App) renderCost(cost float64) footerSegment {
	text := fmt.Sprintf("$%.2f", cost)
	switch {
	case cost < 0.5:
		return footerSegment{text: text, style: footerCostGreen}
	case cost < 1.0:
		return footerSegment{text: text, style: footerCostYellow}
	case cost < 2.0:
		return footerSegment{text: text, style: footerCostOrange}
	default:
		return footerSegment{text: text, style: footerCostRed}
	}
}

func (a *App) renderContextBar(used, total int) footerSegment {
	if total <= 0 {
		total = 1
	}
	pct := float64(used) / float64(total)
	if pct > 1.0 {
		pct = 1.0
	}
	const barWidth = 10
	filled := int(pct * float64(barWidth))
	bar := "[" + strings.Repeat("#", filled) + strings.Repeat("-", barWidth-filled) + "]"
	text := bar + " " + formatTokens(used)

	switch {
	case pct < 0.25:
		return footerSegment{text: text, style: footerContextGreen}
	case pct < 0.5:
		return footerSegment{text: text, style: footerContextYellow}
	case pct < 0.75:
		return footerSegment{text: text, style: footerContextOrange}
	default:
		return footerSegment{text: text, style: footerContextRed}
	}
}

func formatTokens(n int) string {
	return fmt.Sprintf("%.0fk", float64(n)/1000)
}

func (a *App) handleKey(ev *tcell.EventKey, s tcell.Screen) bool {
	switch ev.Key() {
	case tcell.KeyCtrlC:
		return true
	case tcell.KeyCtrlL:
		s.Sync()
		return false
	case tcell.KeyUp:
		a.scrollBy(-1)
		return false
	case tcell.KeyDown:
		a.scrollBy(1)
		return false
	case tcell.KeyPgUp:
		bodyHeight, _, _, _ := a.layoutMetrics()
		a.scrollBy(-bodyHeight)
		return false
	case tcell.KeyPgDn:
		bodyHeight, _, _, _ := a.layoutMetrics()
		a.scrollBy(bodyHeight)
		return false
	}

	if a.working {
		return false
	}

	switch ev.Key() {
	case tcell.KeyEnter:
		if strings.TrimSpace(string(a.input)) != "" {
			a.submitPrompt()
		}
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		a.deleteBeforeCursor()
	case tcell.KeyDelete:
		a.deleteAtCursor()
	case tcell.KeyLeft:
		if a.cursor > 0 {
			a.cursor--
		}
	case tcell.KeyRight:
		if a.cursor < len(a.input) {
			a.cursor++
		}
	case tcell.KeyHome, tcell.KeyCtrlA:
		a.cursor = 0
	case tcell.KeyEnd, tcell.KeyCtrlE:
		a.cursor = len(a.input)
	case tcell.KeyRune:
		a.insertString(ev.Str())
	}

	return false
}

func (a *App) insertRune(r rune) {
	a.input = append(a.input[:a.cursor], append([]rune{r}, a.input[a.cursor:]...)...)
	a.cursor++
}

func (a *App) insertString(text string) {
	for _, r := range text {
		a.insertRune(r)
	}
}

func (a *App) deleteBeforeCursor() {
	if a.cursor == 0 {
		return
	}
	a.input = append(a.input[:a.cursor-1], a.input[a.cursor:]...)
	a.cursor--
}

func (a *App) deleteAtCursor() {
	if a.cursor >= len(a.input) {
		return
	}
	a.input = append(a.input[:a.cursor], a.input[a.cursor+1:]...)
}

func (a *App) scrollBy(delta int) {
	bodyHeight, _, _, _ := a.layoutMetrics()
	maxTop := max(len(a.renderConversationLines(a.width))-bodyHeight, 0)
	a.scrollTop += delta
	if a.scrollTop < 0 {
		a.scrollTop = 0
	}
	if a.scrollTop > maxTop {
		a.scrollTop = maxTop
	}
}

func (a *App) scrollToBottom() {
	bodyHeight, _, _, _ := a.layoutMetrics()
	a.scrollTop = max(len(a.renderConversationLines(max(a.width, 1)))-bodyHeight, 0)
}

func drawBox(s tcell.Screen, x, y, w, h int, style tcell.Style) {
	if w < 2 || h < 2 {
		return
	}
	for dx := 0; dx < w; dx++ {
		r := tcell.RuneHLine
		if dx == 0 {
			r = tcell.RuneULCorner
		} else if dx == w-1 {
			r = tcell.RuneURCorner
		}
		s.Put(x+dx, y, string(r), style)
		r = tcell.RuneHLine
		if dx == 0 {
			r = tcell.RuneLLCorner
		} else if dx == w-1 {
			r = tcell.RuneLRCorner
		}
		s.Put(x+dx, y+h-1, string(r), style)
	}
	for dy := 1; dy < h-1; dy++ {
		s.Put(x, y+dy, string(tcell.RuneVLine), style)
		fillLine(s, x+1, y+dy, w-2, inputTextStyle)
		s.Put(x+w-1, y+dy, string(tcell.RuneVLine), style)
	}
}

func drawStyledInputLine(s tcell.Screen, x, y, width int, text string) {
	if strings.HasPrefix(text, "❯ ") {
		drawText(s, x, y, width, "❯ ", inputPromptStyle)
		drawText(s, x+runewidth.StringWidth("❯ "), y, width-runewidth.StringWidth("❯ "), strings.TrimPrefix(text, "❯ "), inputTextStyle)
		return
	}
	drawText(s, x, y, width, text, inputTextStyle)
}

func drawStyledLine(s tcell.Screen, x, y, width int, line styledLine) int {
	if width <= 0 || y < 0 {
		return 0
	}
	if line.fill {
		fillLine(s, x, y, width, line.fillStyle)
	}
	used := 0
	for _, span := range line.spans {
		if used >= width {
			break
		}
		used += drawText(s, x+used, y, width-used, span.text, span.style)
	}
	return used
}

func drawText(s tcell.Screen, x, y, width int, text string, style tcell.Style) int {
	if width <= 0 || y < 0 {
		return 0
	}
	used := 0
	for _, r := range text {
		rw := runeWidth(r)
		if used+rw > width {
			break
		}
		_, drawn := s.Put(x+used, y, string(r), style)
		if drawn <= 0 {
			drawn = rw
		}
		used += drawn
	}
	return used
}

func fillLine(s tcell.Screen, x, y, width int, style tcell.Style) {
	if width <= 0 || y < 0 {
		return
	}
	for i := 0; i < width; i++ {
		s.Put(x+i, y, " ", style)
	}
}

func wrapPrefixedText(firstPrefix, restPrefix, text string, width int) []string {
	if width < 1 {
		width = 1
	}
	if text == "" {
		return []string{firstPrefix}
	}

	lines := []string{firstPrefix}
	lineWidths := []int{runewidth.StringWidth(firstPrefix)}
	prefixWidths := []int{runewidth.StringWidth(firstPrefix)}
	current := 0

	for _, r := range text {
		if r == '\n' {
			lines = append(lines, restPrefix)
			lineWidths = append(lineWidths, runewidth.StringWidth(restPrefix))
			prefixWidths = append(prefixWidths, runewidth.StringWidth(restPrefix))
			current++
			continue
		}

		rw := runeWidth(r)
		if lineWidths[current]+rw > width && lineWidths[current] > prefixWidths[current] {
			lines = append(lines, restPrefix)
			lineWidths = append(lineWidths, runewidth.StringWidth(restPrefix))
			prefixWidths = append(prefixWidths, runewidth.StringWidth(restPrefix))
			current++
		}

		lines[current] += string(r)
		lineWidths[current] += rw
	}

	return lines
}

func runeWidth(r rune) int {
	w := runewidth.RuneWidth(r)
	if w <= 0 {
		return 1
	}
	return w
}

func runewidthString(s string) int {
	return runewidth.StringWidth(s)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

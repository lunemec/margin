package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/redrick/margin/internal/doc"
	"github.com/redrick/margin/internal/render"
)

var (
	colText      = "#c9d1d9"
	colDim       = "#7d8590"
	colTitle     = "#f0f6fc"
	colHeading   = "#79c0ff"
	colProblem   = "#ff7b72"
	colAdded     = "#3fb950"
	colChanged   = "#d29922"
	colRemoved   = "#f85149"
	colGhost     = "#b08286"
	colCode      = "#ffa657"
	colNit       = "#8b949e"
	colInk       = "#0d1117"
	colBar       = "#161b22"
	colNoteSel   = "#f0f6fc"
	colCursorNum = "#58a6ff"
	colMatch     = "#e3b341"
	colTested    = "#39c5cf"
	colSpotDim   = "#484f58"
	colDecide    = "#d2a8ff"
	colMoved     = "#79c0ff"
	colComment   = "#e3b341"

	tintAdded      = "#12261e"
	tintAddedCur   = "#1d4030"
	tintAddedHL    = "#245c3a"
	tintRemoved    = "#2d1517"
	tintRemovedCur = "#4a2226"
	tintRemovedHL  = "#6e2b30"
	tintCursor     = "#1f2a3a"
	tintNoteSel    = "#1c2433"
	tintMoved      = "#131d2e"
	tintMovedCur   = "#1d2d4a"
)

type kindStyle struct {
	icon, label, color string
}

var kindStyles = makeKindStyles()

func makeKindStyles() map[string]kindStyle {
	return map[string]kindStyle{
		doc.KindIssue:    {"!", "problem", colProblem},
		doc.KindQuestion: {"?", "question", colChanged},
		doc.KindDecide:   {"◆", "your call", colDecide},
		doc.KindOK:       {"✓", "looks right", colAdded},
		doc.KindInfo:     {"i", "context", colHeading},
		doc.KindNit:      {"·", "nit", colNit},
	}
}

// UseTerminalTheme switches to light colours when the terminal has a light background. Call it
// before the program takes over the terminal: it asks the terminal for its background colour.
func UseTerminalTheme() {
	if !lipgloss.HasDarkBackground() {
		useLight()
	}
}

func useLight() {
	colText, colDim, colTitle, colHeading = "#1f2328", "#656d76", "#0d1117", "#0550ae"
	colProblem, colAdded, colChanged, colRemoved = "#cf222e", "#1a7f37", "#9a6700", "#cf222e"
	colGhost, colCode, colNit, colInk, colBar = "#8c5a5f", "#953800", "#6e7781", "#ffffff", "#eaeef2"
	colNoteSel, colCursorNum, colMatch, colTested = "#0d1117", "#0969da", "#bf8700", "#1b7c83"
	colSpotDim, colDecide, colMoved, colComment = "#afb8c1", "#8250df", "#0550ae", "#9a6700"
	tintAdded, tintAddedCur, tintAddedHL = "#e6ffec", "#ccffd8", "#abf2bc"
	tintRemoved, tintRemovedCur, tintRemovedHL = "#ffebe9", "#ffd7d5", "#ffc1bd"
	tintCursor, tintNoteSel, tintMoved, tintMovedCur = "#ddf4ff", "#eef2f7", "#eef4ff", "#dbe9ff"
	tintEditSel, colEditCur, colInsCur = "#b6d7ff", "#bf8700", "#1b7c83"
	kindStyles = makeKindStyles()
	verdicts = makeVerdicts()
	render.Light = true
}

func (m *Model) noteStyle(n *doc.Note) kindStyle {
	if n.Unbacked() {
		return kindStyle{"!?", "problem without evidence", colChanged}
	}
	if s, ok := kindStyles[n.Level()]; ok {
		return s
	}
	return kindStyles[doc.KindInfo]
}

func riskColor(risk string) string {
	switch risk {
	case "high":
		return colProblem
	case "medium":
		return colChanged
	case "low":
		return colAdded
	}
	return colDim
}

// plain drops the light markdown the notes column renders, for one-line summaries.
func plain(s string) string { return strings.NewReplacer("**", "", "`", "").Replace(s) }

type seg struct {
	text, fg, bg string
	bold         bool
}

func (m *Model) bar(segs []seg, width int, right string) string {
	return paintBar(m.paint, segs, width, right)
}

// paintBar renders segments left to right into width cells and right-aligns right while it still fits.
func paintBar(paint *render.Painter, segs []seg, width int, right string) string {
	var b strings.Builder
	avail := width
	if rw := runewidth.StringWidth(right); rw > 0 && width > rw+20 {
		avail = width - rw
	}
	used := 0
	for _, s := range segs {
		w := min(runewidth.StringWidth(s.text), avail-used)
		if w <= 0 {
			break
		}
		b.WriteString(paint.Text(s.text, w, s.fg, s.bg, s.bold, false))
		used += w
	}
	if gap := avail - used; gap > 0 {
		b.WriteString(paint.Text("", gap, "", colBar, false, false))
	}
	if avail < width {
		b.WriteString(paint.Text(right, width-avail, colDim, colBar, false, false))
	}
	return b.String()
}

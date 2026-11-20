package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// refModel opens the example review on store's first note, which is made to mention two notes.
func refModel(t *testing.T) *Model {
	t.Helper()
	m := newModel(t)
	mustGoto(t, m, "store:1")
	m.doc.Stations[m.station].Notes[0].Text = "A nil log is valid, see audit:1 and (reserve:2). Not main.go:3 or audit:9."
	return m
}

func TestFollowMentionAndBack(t *testing.T) {
	m := refModel(t)
	from, cur := m.station, m.cur
	press(m, "L")
	if st := m.doc.Stations[m.station]; st.ID != "audit" || m.note != 0 {
		t.Fatalf("L goes to the first mention: %s note %d", st.ID, m.note)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	if m.station != from || m.note != 0 || m.cur != cur {
		t.Fatalf("ctrl+o comes back: station %d note %d cur %d, want %d 0 %d", m.station, m.note, m.cur, from, cur)
	}
	press(m, "L")
	if st := m.doc.Stations[m.station]; st.ID != "reserve" || m.note != 1 {
		t.Fatalf("L again goes to the next mention: %s note %d", st.ID, m.note)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	if m.station != from {
		t.Fatal("ctrl+o with nothing to go back to stays put")
	}
}

func TestClickMention(t *testing.T) {
	m := refModel(t)
	m.View()
	for line, links := range m.linkAt {
		k := links[0]
		click(m, m.codeWidth()+1+k.x0, line+1, false)
		if m.station != k.station || m.note != k.note {
			t.Fatalf("a click on a mention follows it: station %d note %d, want %d %d", m.station, m.note, k.station, k.note)
		}
		if len(m.jumps) != 1 {
			t.Fatalf("the click is remembered for ctrl+o: %d jumps", len(m.jumps))
		}
		return
	}
	t.Fatal("no mention drawn in the notes column")
}

func TestMentionsOnlyLinkNotes(t *testing.T) {
	m := refModel(t)
	m.View()
	n := 0
	for _, links := range m.linkAt {
		n += len(links)
	}
	if n != 2 {
		t.Fatalf("audit:1 and reserve:2 are links, main.go:3 and audit:9 are not: %d links", n)
	}
}

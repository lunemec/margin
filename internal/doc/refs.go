package doc

import (
	"fmt"
	"regexp"
	"strconv"
)

// Ref is a mention of a note in prose: <stop>:<n>, the stop's id and the note counted from 1, the
// same address `margin goto` takes. Only ids of stops in the review count, so file:line stays text.
type Ref struct {
	// Start and End are byte offsets of the mention in the text.
	Start, End int
	Station    int
	// Note is the index into the stop's notes, or -1 when the stop has no such note.
	Note int
	// N is the note number as written.
	N int
}

// Label is the mention as written.
func (r Ref) Label(d *Doc) string { return fmt.Sprintf("%s:%d", d.Stations[r.Station].ID, r.N) }

var refRe = regexp.MustCompile(`([^\s:<>]+):(\d+)`)

// Refs finds the mentions of notes in s. A mention may sit in punctuation or backticks, as in
// "(see resolve:5)" or "`resolve:5`".
func (d *Doc) Refs(s string) []Ref {
	var out []Ref
	for _, m := range refRe.FindAllStringSubmatchIndex(s, -1) {
		if m[1] < len(s) && isWordByte(s[m[1]]) {
			continue
		}
		head := s[m[2]:m[3]]
		n, err := strconv.Atoi(s[m[4]:m[5]])
		if err != nil {
			continue
		}
		// The longest tail of the word that is a stop id: "(resolve" is "resolve".
		for off := 0; off < len(head); off++ {
			si, st := d.Station(head[off:])
			if st == nil || st.Kind != Code {
				continue
			}
			note := n - 1
			if n < 1 || n > len(st.Notes) {
				note = -1
			}
			out = append(out, Ref{Start: m[2] + off, End: m[1], Station: si, Note: note, N: n})
			break
		}
	}
	return out
}

func isWordByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// noteRefs lists the mentions in a note's own words: its text and its evidence.
func (d *Doc) noteRefs(n *Note) []Ref {
	return append(d.Refs(n.Text), d.Refs(n.Evidence)...)
}

// linkRefs fills every note's RefBy with the notes that mention it.
func (d *Doc) linkRefs() {
	for _, st := range d.Stations {
		for i, n := range st.Notes {
			from := fmt.Sprintf("%s:%d", st.ID, i+1)
			seen := map[*Note]bool{}
			for _, r := range d.noteRefs(n) {
				if r.Note < 0 {
					continue
				}
				to := d.Stations[r.Station].Notes[r.Note]
				if to == n || seen[to] {
					continue
				}
				seen[to] = true
				to.RefBy = append(to.RefBy, from)
			}
		}
	}
}

// BrokenRefs describes each mention in a note that points at a note the stop does not have.
func (d *Doc) BrokenRefs() []string {
	var out []string
	for _, st := range d.Stations {
		for i, n := range st.Notes {
			for _, r := range d.noteRefs(n) {
				if r.Note < 0 {
					to := d.Stations[r.Station]
					out = append(out, fmt.Sprintf("note %s:%d mentions %s, but stop %s has %d notes", st.ID, i+1, r.Label(d), to.ID, len(to.Notes)))
				}
			}
		}
	}
	return out
}

package doc

import (
	"reflect"
	"testing"

	"github.com/redrick/margin/internal/review"
)

func refDoc() *Doc {
	note := func(text string) *Note { return &Note{Note: review.Note{Text: text}} }
	return &Doc{Stations: []*Station{
		{Kind: Overview, ID: "overview"},
		{Kind: Code, ID: "resolve", Notes: []*Note{note("see command:2"), note("plain")}},
		{Kind: Code, ID: "command", Notes: []*Note{note("x"), note("follows (resolve:1) and `resolve:2`, not command:2")}},
		{Kind: Code, ID: "cache.go", Notes: []*Note{note("y")}},
	}}
}

func TestRefsFindOnlyStopIDs(t *testing.T) {
	d := refDoc()
	s := "resolve:2, (command:1); main.go:12, overview:1, resolve:9, resolve:2x, cache.go:1 and resolve:1: done"
	var got []string
	for _, r := range d.Refs(s) {
		got = append(got, s[r.Start:r.End])
		if r.Label(d) != s[r.Start:r.End] {
			t.Errorf("label %q, text %q", r.Label(d), s[r.Start:r.End])
		}
	}
	want := []string{"resolve:2", "command:1", "resolve:9", "cache.go:1", "resolve:1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("refs = %q, want %q", got, want)
	}
	if r := d.Refs("resolve:9")[0]; r.Note != -1 || r.N != 9 {
		t.Errorf("missing note: %+v", r)
	}
	if r := d.Refs("command:2")[0]; r.Station != 2 || r.Note != 1 {
		t.Errorf("command:2 = %+v", r)
	}
}

func TestLinkRefsAndBroken(t *testing.T) {
	d := refDoc()
	d.Stations[2].Notes[1].Evidence = "and resolve:7"
	d.linkRefs()
	resolve, command := d.Stations[1].Notes, d.Stations[2].Notes
	if want := []string{"command:2"}; !reflect.DeepEqual(resolve[0].RefBy, want) || !reflect.DeepEqual(resolve[1].RefBy, want) {
		t.Errorf("resolve RefBy = %v, %v", resolve[0].RefBy, resolve[1].RefBy)
	}
	if want := []string{"resolve:1"}; !reflect.DeepEqual(command[1].RefBy, want) {
		t.Errorf("command:2 RefBy = %v (a note does not refer to itself)", command[1].RefBy)
	}
	want := []string{"note command:2 mentions resolve:7, but stop resolve has 2 notes"}
	if got := d.BrokenRefs(); !reflect.DeepEqual(got, want) {
		t.Errorf("BrokenRefs = %q", got)
	}
}

package main

import (
	"strings"

	"github.com/redrick/margin/internal/doc"
	"github.com/redrick/margin/internal/state"
)

type DiscussionLocation struct {
	Part int `json:"part"`
	At   int `json:"at"`
}

func locateDiscussionLine(d *doc.Doc, q state.Question) DiscussionLocation {
	lost := DiscussionLocation{Part: -1, At: -1}
	_, s := d.Station(q.Station)
	if s == nil || q.Line < 1 {
		return lost
	}
	var parts []int
	for i, p := range s.Parts {
		if p.Err == nil && p.Spec.File == q.File && p.Base == (q.Side == "base") {
			parts = append(parts, i)
		}
	}
	if len(parts) == 0 {
		return lost
	}
	lines := s.Parts[parts[0]].Lines
	at := q.Line - 1
	if at >= len(lines) || !matchesDiscussionLine(lines, at, q.Needle) {
		at = -1
		if q.Needle == "" {
			return lost
		}
		for i, line := range lines {
			if strings.TrimSpace(line) == q.Needle {
				if at >= 0 {
					return lost
				}
				at = i
			}
		}
		if at < 0 {
			return lost
		}
	}
	for _, i := range parts {
		p := s.Parts[i]
		if p.Range.Start <= at && at <= p.Range.End {
			return DiscussionLocation{Part: i, At: at}
		}
	}
	return DiscussionLocation{Part: parts[0], At: at}
}

func matchesDiscussionLine(lines []string, at int, needle string) bool {
	line := strings.TrimSpace(lines[at])
	// A comment on a blank line stores the nearest nonblank line above as its anchor.
	if line == "" && needle != "" {
		for at > 0 && line == "" {
			at--
			line = strings.TrimSpace(lines[at])
		}
	}
	return line == needle
}

package state

import "testing"

func TestCommentLabel(t *testing.T) {
	for _, c := range []struct {
		text     string
		name     string
		blocking bool
	}{
		{"issue (blocking): nil map write", "issue", true},
		{"issue (non-blocking): could be clearer", "issue", false},
		{"suggestion (security, blocking): hash it", "suggestion", true},
		{"praise: lovely test", "praise", false},
		{"just a remark: with a colon", "", false},
		{"why is this here?", "", false},
	} {
		name, blocking := CommentLabel(c.text)
		if name != c.name || blocking != c.blocking {
			t.Errorf("CommentLabel(%q) = %q, %v", c.text, name, blocking)
		}
	}
	if p := Prefix("issue", true); p != "issue (blocking): " {
		t.Errorf("prefix = %q", p)
	}
	if p := Prefix("praise", false); p != "praise: " {
		t.Errorf("prefix = %q", p)
	}
}

func TestCommentBody(t *testing.T) {
	for _, c := range []struct{ text, prefix, body string }{
		{"issue (blocking): nil map write", "issue (blocking): ", "nil map write"},
		{"just a remark: with a colon", "", "just a remark: with a colon"},
		{"plain words", "", "plain words"},
		{"answer to your question (note 2: why keep the cache?): it is hot", "answer to your question (note 2: why keep the cache?): ", "it is hot"},
		{"answer to your question (note 1: x): nitpick: tidy", "answer to your question (note 1: x): nitpick: ", "tidy"},
	} {
		prefix, body := CommentBody(c.text)
		if prefix != c.prefix || body != c.body {
			t.Errorf("CommentBody(%q) = %q, %q", c.text, prefix, body)
		}
	}
}

func TestEditDraft(t *testing.T) {
	s := &State{Questions: []Question{
		{ID: "c1", Kind: KindComment, Draft: true, Text: "question: why here?"},
		{ID: "c2", Kind: KindComment, Text: "sent one"},
	}}
	if err := s.EditDraft("c1", "  why not in the loader?  "); err != nil {
		t.Fatal(err)
	}
	if got := s.Questions[0].Text; got != "question: why not in the loader?" {
		t.Errorf("text = %q", got)
	}
	if err := s.EditDraft("c2", "changed"); err == nil || s.Questions[1].Text != "sent one" {
		t.Errorf("sent comment edited: %v %q", err, s.Questions[1].Text)
	}
	if err := s.EditDraft("c1", " "); err == nil {
		t.Error("empty edit accepted")
	}
	if err := s.EditDraft("c9", "x"); err == nil {
		t.Error("missing comment accepted")
	}
}

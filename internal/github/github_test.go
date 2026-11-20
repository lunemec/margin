package github

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const patch = `@@ -10,6 +10,7 @@ func load() {
 	a := 1
 	b := 2
-	c := a + b
+	c := a * b
+	log(c)

 	return c
 }`

func TestParsePatch(t *testing.T) {
	c := ParsePatch(patch)
	if got := c.Right[12]; got != "\tc := a * b" {
		t.Fatalf("right 12 = %q", got)
	}
	if got := c.Left[12]; got != "\tc := a + b" {
		t.Fatalf("left 12 = %q", got)
	}
	if _, ok := c.Right[16]; !ok {
		t.Fatal("last context line missing")
	}
	if _, ok := c.Right[9]; ok {
		t.Fatal("line before the hunk is not commentable")
	}
	if c.Right[15] != c.Left[14] || c.Right[15] != "\treturn c" {
		t.Fatalf("context lines must be on both sides: %q %q", c.Right[15], c.Left[14])
	}
}

func testPR() *PR {
	return &PR{Number: 7, Owner: "o", Repo: "r", HeadSHA: strings.Repeat("a", 40), Author: "someone", Viewer: "me",
		Files: map[string]*Commentable{"x.go": ParsePatch(patch)}}
}

func TestPlace(t *testing.T) {
	pr := testPR()
	cases := []struct {
		name string
		c    Comment
		want Placement
	}{
		{"on its line", Comment{Path: "x.go", Line: 12, Needle: "c := a * b"}, Placement{Line: 12, Side: "RIGHT", Inline: true, InFile: true}},
		{"old code", Comment{Path: "x.go", Base: true, Line: 12, Needle: "c := a + b"}, Placement{Line: 12, Side: "LEFT", Inline: true, InFile: true}},
		{"moved by local edits", Comment{Path: "x.go", Line: 20, Needle: "log(c)"}, Placement{Line: 13, Side: "RIGHT", Inline: true, InFile: true, Moved: true}},
		{"blank line by text above", Comment{Path: "x.go", Line: 14, Needle: "log(c)"}, Placement{Line: 14, Side: "RIGHT", Inline: true, InFile: true}},
		{"outside the diff", Comment{Path: "x.go", Line: 40, Needle: "nowhere()"}, Placement{Line: 40, Side: "RIGHT", InFile: true}},
		{"not in the PR", Comment{Path: "y.go", Line: 3, Needle: "x"}, Placement{Path: "y.go", Line: 3, Side: "RIGHT"}},
	}
	for _, tc := range cases {
		got := pr.Place(tc.c)
		if tc.want.Path == "" {
			tc.want.Path = "x.go"
		}
		if got.Path != tc.want.Path || got.Line != tc.want.Line || got.Side != tc.want.Side || got.Inline != tc.want.Inline ||
			got.InFile != tc.want.InFile || got.Moved != tc.want.Moved {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestBuildReview(t *testing.T) {
	pr := testPR()
	pls := []Placement{
		{ID: "c1", Path: "x.go", Line: 12, Side: "RIGHT", Inline: true, InFile: true, Body: "inline"},
		{ID: "c2", Path: "x.go", Line: 40, Side: "RIGHT", InFile: true, Body: "far away"},
	}
	p, err := pr.BuildReview(EventRequestChanges, "Please fix.", pls)
	if err != nil {
		t.Fatal(err)
	}
	if p["event"] != EventRequestChanges || p["commit_id"] != pr.HeadSHA {
		t.Fatalf("payload %v", p)
	}
	if cs := p["comments"].([]map[string]any); len(cs) != 1 || cs[0]["line"] != 12 {
		t.Fatalf("comments %v", cs)
	}
	body := p["body"].(string)
	if !strings.HasPrefix(body, "Please fix.") || !strings.Contains(body, "**x.go**, line 40:\n\nfar away") {
		t.Fatalf("body %q", body)
	}
	if _, err := pr.BuildReview(EventComment, "", nil); err == nil {
		t.Fatal("an empty comment review must be refused")
	}
	if _, err := pr.BuildReview(EventApprove, "", nil); err != nil {
		t.Fatalf("a bare approval is fine: %v", err)
	}
	pr.Viewer = "someone"
	if _, err := pr.BuildReview(EventApprove, "", nil); err == nil {
		t.Fatal("approving your own PR must be refused")
	}
	if _, err := pr.BuildReview("MERGE", "x", nil); err == nil {
		t.Fatal("unknown event must be refused")
	}
}

type call struct {
	args  []string
	stdin []byte
}

func fake(calls *[]call, answers map[string]string) Runner {
	return func(_ string, stdin []byte, args ...string) ([]byte, error) {
		*calls = append(*calls, call{args, stdin})
		key := strings.Join(args, " ")
		for k, v := range answers {
			if strings.HasPrefix(key, k) {
				return []byte(v), nil
			}
		}
		return nil, errors.New("unexpected gh " + key)
	}
}

func TestFindAndPost(t *testing.T) {
	head := strings.Repeat("b", 40)
	files, _ := json.Marshal(map[string]string{"filename": "x.go", "patch": patch})
	var calls []call
	run := fake(&calls, map[string]string{
		"pr view 7 --json":  `{"number":7,"title":"T","url":"https://github.com/o/r/pull/7","headRefOid":"` + head + `","state":"OPEN","author":{"login":"someone"}}`,
		"api user":          "me\n",
		"api --paginate":    string(files) + "\n",
		"api --method POST": `{"html_url":"https://github.com/o/r/pull/7#discussion_r1"}`,
	})
	pr, err := Find(run, "/repo", "PR #7: T", "")
	if err != nil {
		t.Fatal(err)
	}
	if pr.Owner != "o" || pr.Repo != "r" || pr.Viewer != "me" || pr.Own() || pr.Files["x.go"] == nil {
		t.Fatalf("pr %+v", pr)
	}
	pl := pr.Place(Comment{Path: "x.go", Line: 40, Needle: "far()", Body: "hm"})
	posted, err := pr.PostComment(run, "/repo", pl)
	if err != nil {
		t.Fatal(err)
	}
	if posted.URL != "https://github.com/o/r/pull/7#discussion_r1" {
		t.Fatalf("url %q", posted.URL)
	}
	last := calls[len(calls)-1]
	if strings.Join(last.args, " ") != "api --method POST repos/o/r/pulls/7/comments --input -" {
		t.Fatalf("args %v", last.args)
	}
	var sent map[string]any
	if err := json.Unmarshal(last.stdin, &sent); err != nil {
		t.Fatal(err)
	}
	if sent["subject_type"] != "file" || sent["body"] != "line 40: hm" || sent["commit_id"] != head {
		t.Fatalf("payload %v", sent)
	}
	if _, err := pr.PostComment(run, "/repo", pr.Place(Comment{Path: "nope.go", Line: 1})); err == nil {
		t.Fatal("a file outside the PR must not be posted")
	}
}

func TestFindByHead(t *testing.T) {
	head := strings.Repeat("c", 40)
	var calls []call
	run := fake(&calls, map[string]string{
		"pr list": `[{"number":3,"url":"https://github.com/o/r/pull/3","headRefOid":"` + strings.Repeat("d", 40) + `"},` +
			`{"number":4,"url":"https://github.com/o/r/pull/4","headRefOid":"` + head + `","author":{"login":"x"}}]`,
		"api user":       "me",
		"api --paginate": "",
	})
	pr, err := Find(run, "/repo", "Branch feature", head)
	if err != nil {
		t.Fatal(err)
	}
	if pr.Number != 4 {
		t.Fatalf("picked #%d", pr.Number)
	}
	run = fake(&calls, map[string]string{"pr list": `[]`})
	if _, err := Find(run, "/repo", "Branch feature", head); err == nil {
		t.Fatal("no PR must be an error")
	}
}

func TestGHError(t *testing.T) {
	err := ghError([]byte(`{"message":"Unprocessable Entity","errors":["Can not approve your own pull request"]}`), "", errors.New("exit 1"))
	if err.Error() != "GitHub: Unprocessable Entity; Can not approve your own pull request" {
		t.Fatal(err)
	}
}

const threadsPage = `{"data":{"repository":{"pullRequest":{"reviewThreads":{"pageInfo":{"hasNextPage":false},"nodes":[
 {"id":"T1","isResolved":false,"viewerCanResolve":true,"viewerCanUnresolve":false,"path":"x.go","comments":{"nodes":[
  {"databaseId":11,"url":"u11","body":"inline words","author":{"login":"me"},"reactionGroups":[{"content":"THUMBS_UP","reactors":{"totalCount":2}},{"content":"EYES","reactors":{"totalCount":0}}]},
  {"databaseId":12,"url":"u12","body":"done","author":null,"reactionGroups":[]}]}},
 {"id":"T2","isResolved":true,"viewerCanResolve":false,"viewerCanUnresolve":true,"path":"x.go","comments":{"nodes":[
  {"databaseId":21,"url":"u21","body":"line 40: from a review","author":{"login":"me"},"reactionGroups":[]}]}}]}}}}}`

func TestThreads(t *testing.T) {
	var calls []call
	pr := testPR()
	run := fake(&calls, map[string]string{"api graphql --paginate": threadsPage + "\n"})
	ts, err := pr.Threads(run, "/repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 2 || ts[0].Comments[0].Reactions[0] != (Reaction{"THUMBS_UP", 2}) || len(ts[0].Comments[0].Reactions) != 1 ||
		ts[0].Comments[1].Author != "" || !ts[1].Resolved || !ts[1].CanUnresolve {
		t.Fatalf("threads %+v", ts)
	}
	if got := FindThread(ts, "https://github.com/o/r/pull/7#discussion_r11", "x.go", ""); got != ts[0] {
		t.Fatal("not found by id")
	}
	if got := FindThread(ts, "https://github.com/o/r/pull/7#discussion_r99", "x.go", "inline words"); got != nil {
		t.Fatal("an id that is gone must not fall back to text")
	}
	if got := FindThread(ts, "https://github.com/o/r/pull/7#pullrequestreview-5", "x.go", "from a review"); got != ts[1] {
		t.Fatal("not found by text")
	}
	if got := FindThread(ts, "https://github.com/o/r/pull/7#pullrequestreview-5", "y.go", "from a review"); got != nil {
		t.Fatal("matched another file")
	}
	run = fake(&calls, map[string]string{"api graphql -f id=T1": `{"data":{}}`})
	if err := pr.SetResolved(run, "/repo", "T1", true); err != nil {
		t.Fatal(err)
	}
	if q := calls[len(calls)-1].args; !strings.Contains(q[len(q)-1], "resolveReviewThread") || strings.Contains(q[len(q)-1], "unresolve") {
		t.Fatalf("mutation %v", q)
	}
}

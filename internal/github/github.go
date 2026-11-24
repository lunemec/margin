// Package github posts the reader's own review comments to a GitHub pull request through gh. It
// only ever runs when the reader asked for it in the viewer and confirmed what goes out; nothing in
// margin posts on its own, and the agent has no command that reaches it.
package github

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Runner runs gh with args and stdin, returning its stdout. Tests swap it for a fake.
type Runner func(dir string, stdin []byte, args ...string) ([]byte, error)

// GH runs the real gh, never prompting and never paging.
func GH(dir string, stdin []byte, args ...string) ([]byte, error) {
	gh, err := exec.LookPath("gh")
	if err != nil {
		return nil, errors.New("install the GitHub CLI gh and log in with gh auth login")
	}
	cmd := exec.Command(gh, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_PAGER=cat", "NO_COLOR=1")
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, ghError(out, stderr.String(), err)
	}
	return out, nil
}

// ghError turns a failed gh api call into what GitHub said, which is the useful part.
func ghError(out []byte, stderr string, err error) error {
	var apiErr struct {
		Message string `json:"message"`
		Errors  []any  `json:"errors"`
	}
	if json.Unmarshal(out, &apiErr) == nil && apiErr.Message != "" {
		msg := apiErr.Message
		for _, e := range apiErr.Errors {
			switch v := e.(type) {
			case string:
				msg += "; " + v
			case map[string]any:
				if m, ok := v["message"].(string); ok {
					msg += "; " + m
				}
			}
		}
		return errors.New("GitHub: " + msg)
	}
	if s := strings.TrimSpace(stderr); s != "" {
		return errors.New(s)
	}
	return err
}

// PR is the pull request a review's comments go to.
type PR struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	URL     string `json:"url"`
	Owner   string `json:"owner"`
	Repo    string `json:"repo"`
	HeadSHA string `json:"headSha"`
	Author  string `json:"author"`
	// Viewer is who gh is logged in as: the comments are posted under this name.
	Viewer string `json:"viewer"`
	State  string `json:"state"`
	// Files holds the lines GitHub accepts inline comments on, per path.
	Files map[string]*Commentable `json:"-"`
}

// Own says whether the reader opened the pull request; GitHub refuses to let them approve it or
// request changes on it.
func (p *PR) Own() bool { return p.Viewer != "" && strings.EqualFold(p.Viewer, p.Author) }

var titlePR = regexp.MustCompile(`^PR #(\d+):`)
var urlRe = regexp.MustCompile(`^https://[^/]+/([^/]+)/([^/]+)/pull/(\d+)/?$`)

// Find works out the pull request a review belongs to: the one named in a review of a PR, else the
// open one whose head is the reviewed commit, else the one for the branch checked out in repo.
func Find(run Runner, repo, title, headSHA string) (*PR, error) {
	const fields = "number,title,url,headRefOid,author,state"
	var raw struct {
		Number     int    `json:"number"`
		Title      string `json:"title"`
		URL        string `json:"url"`
		HeadRefOid string `json:"headRefOid"`
		State      string `json:"state"`
		Author     struct {
			Login string `json:"login"`
		} `json:"author"`
	}
	switch {
	case titlePR.MatchString(title):
		n := titlePR.FindStringSubmatch(title)[1]
		out, err := run(repo, nil, "pr", "view", n, "--json", fields)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(out, &raw); err != nil {
			return nil, err
		}
	case headSHA != "":
		out, err := run(repo, nil, "pr", "list", "--state", "open", "--search", headSHA, "--json", fields)
		if err != nil {
			return nil, err
		}
		var list []json.RawMessage
		if err := json.Unmarshal(out, &list); err != nil {
			return nil, err
		}
		for _, m := range list {
			if err := json.Unmarshal(m, &raw); err == nil && raw.HeadRefOid == headSHA {
				break
			}
			raw.Number = 0
		}
		if raw.Number == 0 {
			return nil, fmt.Errorf("no open pull request has %s as its head", short(headSHA))
		}
	default:
		out, err := run(repo, nil, "pr", "view", "--json", fields)
		if err != nil {
			return nil, fmt.Errorf("no pull request for the checked-out branch: %w", err)
		}
		if err := json.Unmarshal(out, &raw); err != nil {
			return nil, err
		}
	}
	m := urlRe.FindStringSubmatch(raw.URL)
	if m == nil {
		return nil, fmt.Errorf("gh returned an unexpected pull request URL %q", raw.URL)
	}
	if !shaRe.MatchString(raw.HeadRefOid) {
		return nil, fmt.Errorf("gh returned an unexpected head commit %q", raw.HeadRefOid)
	}
	pr := &PR{Number: raw.Number, Title: raw.Title, URL: raw.URL, Owner: m[1], Repo: m[2], HeadSHA: raw.HeadRefOid,
		Author: raw.Author.Login, State: raw.State}
	if out, err := run(repo, nil, "api", "user", "--jq", ".login"); err == nil {
		pr.Viewer = strings.TrimSpace(string(out))
	}
	files, err := listFiles(run, repo, pr)
	if err != nil {
		return nil, err
	}
	pr.Files = files
	return pr, nil
}

var shaRe = regexp.MustCompile(`^[0-9a-f]{40,64}$`)

func short(sha string) string {
	if len(sha) > 9 {
		return sha[:9]
	}
	return sha
}

func (p *PR) api(path string) string {
	return fmt.Sprintf("repos/%s/%s/pulls/%d%s", p.Owner, p.Repo, p.Number, path)
}

func listFiles(run Runner, repo string, pr *PR) (map[string]*Commentable, error) {
	out, err := run(repo, nil, "api", "--paginate", pr.api("/files?per_page=100"), "--jq", ".[] | {filename, previous_filename, patch}")
	if err != nil {
		return nil, fmt.Errorf("reading the files of PR #%d: %w", pr.Number, err)
	}
	files := map[string]*Commentable{}
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var f struct {
			Filename string `json:"filename"`
			Previous string `json:"previous_filename"`
			Patch    string `json:"patch"`
		}
		if err := dec.Decode(&f); err != nil {
			return nil, err
		}
		c := ParsePatch(f.Patch)
		files[f.Filename] = c
		if f.Previous != "" {
			c.Previous = f.Previous
		}
	}
	return files, nil
}

// Commentable is the part of one file GitHub shows in the pull request's diff: inline comments can
// only go on these lines. Right is the new file, Left the old one; both map line number to text.
type Commentable struct {
	Right, Left map[int]string
	Previous    string
}

// ParsePatch reads the lines of a unified diff patch GitHub accepts comments on.
func ParsePatch(patch string) *Commentable {
	c := &Commentable{Right: map[int]string{}, Left: map[int]string{}}
	var oldN, newN int
	for _, l := range strings.Split(patch, "\n") {
		if strings.HasPrefix(l, "@@") {
			oldN, newN = hunkStart(l)
			continue
		}
		if oldN == 0 && newN == 0 {
			continue
		}
		switch {
		case strings.HasPrefix(l, "+"):
			c.Right[newN] = l[1:]
			newN++
		case strings.HasPrefix(l, "-"):
			c.Left[oldN] = l[1:]
			oldN++
		case strings.HasPrefix(l, " "), l == "":
			text := strings.TrimPrefix(l, " ")
			c.Right[newN], c.Left[oldN] = text, text
			newN++
			oldN++
		}
	}
	return c
}

var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

func hunkStart(l string) (int, int) {
	m := hunkRe.FindStringSubmatch(l)
	if m == nil {
		return 0, 0
	}
	o, _ := strconv.Atoi(m[1])
	n, _ := strconv.Atoi(m[2])
	return o, n
}

// Comment is one of the reader's comments, as margin keeps it.
type Comment struct {
	ID   string
	Path string
	// Base is set for a comment on the old code.
	Base   bool
	Line   int
	Needle string
	Body   string
}

// Placement is where a comment lands on GitHub. Inline comments sit on their line in the diff; the
// others go on the file, or into the review's summary, with the line named in the text.
type Placement struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Side   string `json:"side"`
	Inline bool   `json:"inline"`
	// InFile is set when the file is part of the pull request, so a comment can at least go on it.
	InFile bool   `json:"inFile"`
	Moved  bool   `json:"moved"`
	Why    string `json:"why,omitempty"`
	Body   string `json:"body"`
}

// Place finds the comment's line in the pull request's diff, by its text first: the local copy may
// not be what was pushed, so the number alone can point at another line.
func (p *PR) Place(c Comment) Placement {
	pl := Placement{ID: c.ID, Path: c.Path, Line: c.Line, Side: "RIGHT", Body: c.Body}
	if c.Base {
		pl.Side = "LEFT"
	}
	f := p.Files[c.Path]
	if f == nil {
		for _, g := range p.Files {
			if c.Base && g.Previous == c.Path {
				f = g
			}
		}
		for name, g := range p.Files {
			if g == f {
				pl.Path = name
			}
		}
	}
	if f == nil {
		pl.Why = fmt.Sprintf("%s is not part of PR #%d, so this goes into the review text", c.Path, p.Number)
		return pl
	}
	pl.InFile = true
	lines := f.Right
	if c.Base {
		lines = f.Left
	}
	needle := strings.TrimSpace(c.Needle)
	at := func(n int) bool {
		t, ok := lines[n]
		if !ok {
			return false
		}
		if needle == "" || strings.TrimSpace(t) == needle {
			return true
		}
		// A blank line is anchored by the nearest text above it.
		if strings.TrimSpace(t) != "" {
			return false
		}
		for k := n - 1; k > 0; k-- {
			if u, ok := lines[k]; !ok || strings.TrimSpace(u) != "" {
				return ok && strings.TrimSpace(u) == needle
			}
		}
		return false
	}
	if at(c.Line) {
		pl.Inline = true
		return pl
	}
	best := 0
	for n := range lines {
		if strings.TrimSpace(lines[n]) == needle && needle != "" && (best == 0 || abs(n-c.Line) < abs(best-c.Line)) {
			best = n
		}
	}
	if best > 0 {
		pl.Line, pl.Inline, pl.Moved = best, true, true
		pl.Why = fmt.Sprintf("line %d here is line %d in the pull request", c.Line, best)
		return pl
	}
	pl.Why = fmt.Sprintf("line %d is outside the part of %s the pull request shows, so it goes on the file", c.Line, pl.Path)
	return pl
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Posted is what GitHub answered: the page to see the comment or review on.
type Posted struct {
	URL string `json:"url"`
}

// PostComment posts one comment at once, on its line when the diff shows it, else on its file.
func (p *PR) PostComment(run Runner, repo string, pl Placement) (*Posted, error) {
	if !pl.InFile {
		return nil, errors.New(pl.Why)
	}
	payload := map[string]any{"body": pl.Body, "commit_id": p.HeadSHA, "path": pl.Path}
	if pl.Inline {
		payload["line"], payload["side"] = pl.Line, pl.Side
	} else {
		payload["subject_type"] = "file"
		payload["body"] = lineRef(pl) + pl.Body
	}
	return p.post(run, repo, "/comments", payload)
}

// Events a review can end with on GitHub.
const (
	EventComment        = "COMMENT"
	EventApprove        = "APPROVE"
	EventRequestChanges = "REQUEST_CHANGES"
)

// BuildReview makes the body of one pull request review: the summary, the decision and the
// comments, inline where GitHub allows it and listed in the summary where not.
func (p *PR) BuildReview(event, summary string, pls []Placement) (map[string]any, error) {
	if event != EventComment && event != EventApprove && event != EventRequestChanges {
		return nil, fmt.Errorf("unknown review decision %q", event)
	}
	if event != EventComment && p.Own() {
		return nil, errors.New("GitHub does not let you approve or request changes on your own pull request; send it as a comment")
	}
	body := strings.TrimSpace(summary)
	comments := []map[string]any{}
	var loose []string
	for _, pl := range pls {
		if pl.Inline {
			comments = append(comments, map[string]any{"path": pl.Path, "line": pl.Line, "side": pl.Side, "body": pl.Body})
			continue
		}
		loose = append(loose, "**"+pl.Path+"**, "+strings.TrimSuffix(lineRef(pl), ": ")+":\n\n"+pl.Body)
	}
	if len(loose) > 0 {
		if body != "" {
			body += "\n\n---\n\n"
		}
		body += strings.Join(loose, "\n\n---\n\n")
	}
	if event == EventRequestChanges && body == "" && len(comments) == 0 {
		return nil, errors.New("say what needs to change: write a summary or add comments")
	}
	if event == EventComment && body == "" && len(comments) == 0 {
		return nil, errors.New("nothing to send: write a summary or add comments")
	}
	return map[string]any{"commit_id": p.HeadSHA, "event": event, "body": body, "comments": comments}, nil
}

// SubmitReview posts the review built by BuildReview.
func (p *PR) SubmitReview(run Runner, repo string, payload map[string]any) (*Posted, error) {
	return p.post(run, repo, "/reviews", payload)
}

func lineRef(pl Placement) string {
	if pl.Side == "LEFT" {
		return fmt.Sprintf("old line %d: ", pl.Line)
	}
	return fmt.Sprintf("line %d: ", pl.Line)
}

func (p *PR) post(run Runner, repo, path string, payload map[string]any) (*Posted, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	out, err := run(repo, data, "api", "--method", "POST", p.api(path), "--input", "-")
	if err != nil {
		return nil, err
	}
	var res struct {
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("GitHub took it, but its answer was unreadable: %w", err)
	}
	return &Posted{URL: res.HTMLURL}, nil
}

// Thread is a review thread on the pull request as GitHub has it now: whether it is resolved, whether
// the reader may change that, and what people said and reacted to it.
type Thread struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	// Line is the thread's line in the pull request's head; 0 when GitHub lost it (outdated) or the
	// thread is on the whole file. OriginalLine is where it was first written.
	Line         int        `json:"line"`
	OriginalLine int        `json:"originalLine"`
	Side         string     `json:"side"`
	Outdated     bool       `json:"outdated"`
	OnFile       bool       `json:"onFile"`
	Resolved     bool       `json:"resolved"`
	CanResolve   bool       `json:"canResolve"`
	CanUnresolve bool       `json:"canUnresolve"`
	Comments     []TComment `json:"comments"`
}

type TComment struct {
	ID        int64      `json:"id"`
	URL       string     `json:"url"`
	Author    string     `json:"author"`
	Body      string     `json:"body"`
	Created   string     `json:"created"`
	Reactions []Reaction `json:"reactions"`
}

// Reaction is one kind of emoji reaction and how many people gave it: THUMBS_UP, HEART, EYES…
type Reaction struct {
	Content string `json:"content"`
	Count   int    `json:"count"`
}

const threadsQuery = `query($owner: String!, $repo: String!, $number: Int!, $endCursor: String) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      reviewThreads(first: 100, after: $endCursor) {
        pageInfo { hasNextPage endCursor }
        nodes {
          id isResolved viewerCanResolve viewerCanUnresolve path line originalLine diffSide isOutdated subjectType
          comments(first: 100) {
            nodes { databaseId url body createdAt author { login } reactionGroups { content reactors { totalCount } } }
          }
        }
      }
    }
  }
}`

// Threads reads every review thread of the pull request; it only reads.
func (p *PR) Threads(run Runner, repo string) ([]*Thread, error) {
	out, err := run(repo, nil, "api", "graphql", "--paginate", "-f", "owner="+p.Owner, "-f", "repo="+p.Repo,
		"-F", "number="+strconv.Itoa(p.Number), "-f", "query="+threadsQuery)
	if err != nil {
		return nil, fmt.Errorf("reading the threads of PR #%d: %w", p.Number, err)
	}
	var threads []*Thread
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var page struct {
			Data struct {
				Repository struct {
					PullRequest struct {
						ReviewThreads struct {
							Nodes []struct {
								ID                 string `json:"id"`
								Line               *int   `json:"line"`
								OriginalLine       *int   `json:"originalLine"`
								DiffSide           string `json:"diffSide"`
								IsOutdated         bool   `json:"isOutdated"`
								SubjectType        string `json:"subjectType"`
								IsResolved         bool   `json:"isResolved"`
								ViewerCanResolve   bool   `json:"viewerCanResolve"`
								ViewerCanUnresolve bool   `json:"viewerCanUnresolve"`
								Path               string `json:"path"`
								Comments           struct {
									Nodes []struct {
										DatabaseID int64  `json:"databaseId"`
										URL        string `json:"url"`
										Body       string `json:"body"`
										CreatedAt  string `json:"createdAt"`
										Author     *struct {
											Login string `json:"login"`
										} `json:"author"`
										ReactionGroups []struct {
											Content  string `json:"content"`
											Reactors struct {
												TotalCount int `json:"totalCount"`
											} `json:"reactors"`
										} `json:"reactionGroups"`
									} `json:"nodes"`
								} `json:"comments"`
							} `json:"nodes"`
						} `json:"reviewThreads"`
					} `json:"pullRequest"`
				} `json:"repository"`
			} `json:"data"`
		}
		if err := dec.Decode(&page); err != nil {
			return nil, err
		}
		for _, n := range page.Data.Repository.PullRequest.ReviewThreads.Nodes {
			t := &Thread{ID: n.ID, Path: n.Path, Side: n.DiffSide, Outdated: n.IsOutdated, OnFile: n.SubjectType == "FILE",
				Resolved: n.IsResolved, CanResolve: n.ViewerCanResolve, CanUnresolve: n.ViewerCanUnresolve}
			if n.Line != nil {
				t.Line = *n.Line
			}
			if n.OriginalLine != nil {
				t.OriginalLine = *n.OriginalLine
			}
			for _, c := range n.Comments.Nodes {
				tc := TComment{ID: c.DatabaseID, URL: c.URL, Body: c.Body, Created: c.CreatedAt, Reactions: []Reaction{}}
				if c.Author != nil {
					tc.Author = c.Author.Login
				}
				for _, g := range c.ReactionGroups {
					if g.Reactors.TotalCount > 0 {
						tc.Reactions = append(tc.Reactions, Reaction{Content: g.Content, Count: g.Reactors.TotalCount})
					}
				}
				t.Comments = append(t.Comments, tc)
			}
			threads = append(threads, t)
		}
	}
	return threads, nil
}

var discussionRe = regexp.MustCompile(`#discussion_r(\d+)$`)

// FindThread finds the thread a posted comment started: by the comment's id in the page GitHub gave
// for it, else, for comments posted inside a review, by its file and the text the reader wrote.
func FindThread(threads []*Thread, url, path, body string) *Thread {
	if m := discussionRe.FindStringSubmatch(url); m != nil {
		id, _ := strconv.ParseInt(m[1], 10, 64)
		for _, t := range threads {
			if len(t.Comments) > 0 && t.Comments[0].ID == id {
				return t
			}
		}
		return nil
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	for _, t := range threads {
		if len(t.Comments) > 0 && t.Path == path && strings.HasSuffix(strings.TrimSpace(t.Comments[0].Body), body) {
			return t
		}
	}
	return nil
}

// SetResolved resolves a review thread, or opens it again. GitHub decides whether the reader may.
func (p *PR) SetResolved(run Runner, repo, threadID string, resolved bool) error {
	op := "unresolveReviewThread"
	if resolved {
		op = "resolveReviewThread"
	}
	q := fmt.Sprintf(`mutation($id: ID!) { %s(input: {threadId: $id}) { thread { isResolved } } }`, op)
	_, err := run(repo, nil, "api", "graphql", "-f", "id="+threadID, "-f", "query="+q)
	return err
}

// Post is something said on the pull request outside the code: a conversation comment, or the
// text of a review with its decision (State: APPROVED, CHANGES_REQUESTED, COMMENTED…).
type Post struct {
	URL       string     `json:"url"`
	Author    string     `json:"author"`
	Body      string     `json:"body"`
	Created   string     `json:"created"`
	State     string     `json:"state,omitempty"`
	Reactions []Reaction `json:"reactions"`
}

const conversationQuery = `query($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      comments(last: 100) { nodes { url body createdAt author { login } reactionGroups { content reactors { totalCount } } } }
      reviews(last: 100) { nodes { url body state submittedAt author { login } reactionGroups { content reactors { totalCount } } } }
    }
  }
}`

// Conversation reads the pull request's conversation comments and the reviews that say something
// or decide something, oldest first; it only reads.
func (p *PR) Conversation(run Runner, repo string) ([]Post, error) {
	out, err := run(repo, nil, "api", "graphql", "-f", "owner="+p.Owner, "-f", "repo="+p.Repo,
		"-F", "number="+strconv.Itoa(p.Number), "-f", "query="+conversationQuery)
	if err != nil {
		return nil, fmt.Errorf("reading the conversation of PR #%d: %w", p.Number, err)
	}
	type node struct {
		URL         string `json:"url"`
		Body        string `json:"body"`
		CreatedAt   string `json:"createdAt"`
		SubmittedAt string `json:"submittedAt"`
		State       string `json:"state"`
		Author      *struct {
			Login string `json:"login"`
		} `json:"author"`
		ReactionGroups []struct {
			Content  string `json:"content"`
			Reactors struct {
				TotalCount int `json:"totalCount"`
			} `json:"reactors"`
		} `json:"reactionGroups"`
	}
	var res struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					Comments struct{ Nodes []node } `json:"comments"`
					Reviews  struct{ Nodes []node } `json:"reviews"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, err
	}
	toPost := func(n node) Post {
		p := Post{URL: n.URL, Body: n.Body, Created: n.CreatedAt, State: n.State, Reactions: []Reaction{}}
		if n.SubmittedAt != "" {
			p.Created = n.SubmittedAt
		}
		if n.Author != nil {
			p.Author = n.Author.Login
		}
		for _, g := range n.ReactionGroups {
			if g.Reactors.TotalCount > 0 {
				p.Reactions = append(p.Reactions, Reaction{Content: g.Content, Count: g.Reactors.TotalCount})
			}
		}
		return p
	}
	posts := []Post{}
	for _, n := range res.Data.Repository.PullRequest.Comments.Nodes {
		posts = append(posts, toPost(n))
	}
	for _, n := range res.Data.Repository.PullRequest.Reviews.Nodes {
		// A review that only carries line comments says nothing of its own.
		if strings.TrimSpace(htmlComment.ReplaceAllString(n.Body, "")) == "" && (n.State == "COMMENTED" || n.State == "PENDING") {
			continue
		}
		posts = append(posts, toPost(n))
	}
	slices.SortStableFunc(posts, func(a, b Post) int { return strings.Compare(a.Created, b.Created) })
	return posts, nil
}

var htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)

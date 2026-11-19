package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/redrick/margin/internal/config"
	"github.com/redrick/margin/internal/control"
	"github.com/redrick/margin/internal/review"
)

type AgentResumeStatus struct {
	Name    string `json:"name,omitempty"`
	Saved   bool   `json:"saved"`
	Exact   bool   `json:"exact"`
	Auto    bool   `json:"auto"`
	CanLink bool   `json:"canLink"`
	Failed  bool   `json:"failed"`
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var opencodeIDPattern = regexp.MustCompile(`^ses_[A-Za-z0-9_-]{1,100}$`)

func agentPreset(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	if fields[len(fields)-1] == "{prompt}" {
		fields = fields[:len(fields)-1]
	}
	if len(fields) == 0 {
		return ""
	}
	name := filepath.Base(fields[0])
	switch name {
	case "claude", "codex":
		if len(fields) == 1 {
			return name
		}
	case "gemini":
		if len(fields) == 2 && (fields[1] == "-i" || fields[1] == "--prompt-interactive") {
			return name
		}
	case "opencode":
		if len(fields) == 2 && fields[1] == "--prompt" {
			return name
		}
	}
	return ""
}

func validSessionID(provider, id string) bool {
	switch provider {
	case "claude", "codex", "gemini":
		return uuidPattern.MatchString(id)
	case "opencode":
		return opencodeIDPattern.MatchString(id)
	default:
		return false
	}
}

func nativeResumeArgs(saved savedAgent) ([]string, error) {
	provider := agentPreset(saved.Command)
	if !validSessionID(provider, saved.ID) {
		return nil, errors.New("there is no linked conversation for this agent")
	}
	exe := strings.Fields(saved.Command)[0]
	switch provider {
	case "codex":
		return []string{exe, "resume", saved.ID}, nil
	case "opencode":
		return []string{exe, "--session", saved.ID}, nil
	default:
		return []string{exe, "--resume", saved.ID}, nil
	}
}

func newClaudeID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (a *Agents) ResumeStatus(path, repo string) AgentResumeStatus {
	saved := readReviewResume(path).Agent
	if saved == nil || saved.Repo != resumeIdentity(repo) || strings.TrimSpace(saved.Command) == "" {
		return AgentResumeStatus{CanLink: agentPreset(config.Load().Agent) != ""}
	}
	name := filepath.Base(strings.Fields(saved.Command)[0])
	provider := agentPreset(saved.Command)
	exact := validSessionID(provider, saved.ID)
	return AgentResumeStatus{
		Name: name, Saved: true, Exact: exact, Auto: saved.Auto && exact && !saved.Failed,
		CanLink: provider != "", Failed: saved.Failed,
	}
}

func (a *Agents) LinkConversation(path, repo, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if r := a.sessions[path]; r != nil && !r.done {
		return errors.New("stop this review's agent before linking a conversation")
	}
	rv, err := loadAgentReview(path, repo)
	if err != nil {
		return err
	}
	lease, err := leaseAgent(path)
	if err != nil {
		return err
	}
	defer lease.Close()
	id = strings.TrimSpace(id)
	return lockResume(func() error {
		record := readReviewResume(path)
		saved := record.Agent
		if saved == nil || saved.Repo != resumeIdentity(rv.RepoDir()) {
			saved = &savedAgent{Command: config.Load().Agent, Repo: resumeIdentity(rv.RepoDir())}
		}
		provider := agentPreset(saved.Command)
		if !validSessionID(provider, id) {
			return errors.New("enter a full conversation ID for the saved agent")
		}
		saved.ID, saved.Run, saved.Auto, saved.Failed = id, rand.Text(), false, false
		record.Agent = saved
		return writeResumeJSON(resumePath(path), record)
	})
}

func loadAgentReview(path, repo string) (*review.Review, error) {
	if !filepath.IsAbs(path) || !filepath.IsAbs(repo) {
		return nil, errors.New("open a review before starting its agent")
	}
	rv, err := review.Load(path)
	if err != nil {
		return nil, err
	}
	if resumeIdentity(repo) != resumeIdentity(rv.RepoDir()) {
		return nil, errors.New("the review's worktree changed; reopen the review")
	}
	return rv, nil
}

func (a *Agents) Resume(path, repo string, cols, rows int) (string, error) {
	return a.restore(path, repo, cols, rows, false)
}

func (a *Agents) Continue(path, repo string, cols, rows int) (string, error) {
	return a.restore(path, repo, cols, rows, true)
}

func (a *Agents) restore(path, repo string, cols, rows int, continued bool) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if s := a.sessions[path]; s != nil && !s.done {
		return s.replay(), nil
	}
	rv, err := loadAgentReview(path, repo)
	if err != nil {
		return "", err
	}
	saved := readReviewResume(path).Agent
	if saved == nil || strings.TrimSpace(saved.Command) == "" || saved.Repo != resumeIdentity(rv.RepoDir()) {
		return "", errors.New("no saved agent for this review")
	}
	if continued {
		return "", a.launchNew(path, repo, saved.Command, false, true, cols, rows)
	}
	args, err := nativeResumeArgs(*saved)
	if err != nil {
		return "", err
	}
	previousRun := saved.Run
	saved.Auto, saved.Failed, saved.Run = true, false, rand.Text()
	if err := a.launch(path, repo, args, *saved, nil, true, cols, rows); err != nil {
		if !errors.Is(err, errAgentInUse) {
			_ = updateReviewResume(path, func(record *reviewResume) bool {
				if record.Agent == nil || record.Agent.Run != previousRun {
					return false
				}
				record.Agent.Auto, record.Agent.Failed = false, true
				return true
			})
		}
		return "", err
	}
	return "", nil
}

// A dedicated socket stays reachable while the reader switches to another review.
func (a *Agents) codexRegistration(path, run string) (*control.Server, string, error) {
	key := rand.Text()
	socket, err := control.AgentSocket(key)
	if err != nil {
		return nil, "", err
	}
	server, err := control.Serve(socket, func(req control.Request) control.Response {
		a.mu.Lock()
		defer a.mu.Unlock()
		s := a.sessions[path]
		fields := strings.SplitN(req.Arg, " ", 3)
		if req.Cmd != "register" || len(fields) != 3 || fields[0] != key ||
			!validSessionID("codex", fields[1]) || resumeIdentity(fields[2]) != resumeIdentity(path) ||
			s == nil || s.run != run || s.registration == nil || s.done || s.stopping {
			return control.Fail(errors.New("this agent launch cannot register a conversation"))
		}
		if err := updateReviewResume(path, func(saved *reviewResume) bool {
			if saved.Agent == nil || saved.Agent.Run != run || saved.Agent.ID != "" {
				return false
			}
			saved.Agent.ID = fields[1]
			return true
		}); err != nil {
			return control.Fail(err)
		}
		s.registration.Close()
		s.registration = nil
		return control.OK(nil)
	})
	if err != nil {
		return nil, "", err
	}
	return server, socket + ":" + key, nil
}

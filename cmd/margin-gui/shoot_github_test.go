//go:build shoot

package main

import (
	"errors"
	"os"
	"strings"
)

// The screenshot server never talks to GitHub. Its pull request is invented, and only exists while
// the file named by MARGIN_SHOOT_GH exists, so shoot-gui.sh can switch it on for the GitHub shots.
func init() {
	ghRun = func(_ string, _ []byte, args ...string) ([]byte, error) {
		if f := os.Getenv("MARGIN_SHOOT_GH"); f == "" || !exists(f) {
			return nil, errors.New("no pull request")
		}
		cmd := strings.Join(args, " ")
		switch {
		case strings.HasPrefix(cmd, "pr view"):
			return []byte(`{"number":42,"title":"Stock reservations get an audit trail","url":"https://github.com/example/shop/pull/42",` +
				`"headRefOid":"` + strings.Repeat("a", 40) + `","state":"OPEN","author":{"login":"sam"}}`), nil
		case cmd == "api user --jq .login":
			return []byte("you\n"), nil
		case strings.HasPrefix(cmd, "api --paginate"):
			return []byte(shootFiles), nil
		case strings.HasPrefix(cmd, "api graphql --paginate"):
			return []byte(shootThreads), nil
		case strings.HasPrefix(cmd, "api graphql -f owner="):
			return []byte(shootConversation), nil
		}
		return nil, errors.New("the screenshot server does not post")
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

const shootFiles = `{"filename":"inventory/stock.go","patch":"@@ -24,7 +30,15 @@\n // Reserve takes qty units of sku out of stock, or fails without changing anything.\n func (s *Store) Reserve(sku string, qty int) error {\n+\tif qty <= 0 {\n+\t\treturn ErrInvalidQuantity\n+\t}\n \ts.mu.Lock()\n \tdefer s.mu.Unlock()\n+\tavailable := s.items[sku]\n+\tif available < qty {\n+\t\treturn fmt.Errorf(\"%w: %s has %d, want %d\", ErrInsufficient, sku, available, qty)\n \t}\n+\ts.items[sku] = available - qty\n+\ts.log.Record(sku, -qty)\n \treturn nil\n }"}`

const shootThreads = `{"data":{"repository":{"pullRequest":{"reviewThreads":{"nodes":[
{"id":"T1","viewerCanResolve":true,"path":"inventory/stock.go","line":37,"diffSide":"RIGHT","comments":{"nodes":[
 {"databaseId":1,"url":"https://github.com/example/shop/pull/42#discussion_r1","body":"praise: Reading it once under the lock is right.","createdAt":"2026-11-19T09:00:00Z","author":{"login":"you"},"reactionGroups":[{"content":"THUMBS_UP","reactors":{"totalCount":2}}]},
 {"databaseId":2,"url":"https://github.com/example/shop/pull/42#discussion_r2","body":"Thanks, that was the point of the change.","createdAt":"2026-11-19T10:00:00Z","author":{"login":"sam"},"reactionGroups":[]}]}},
{"id":"T2","viewerCanResolve":true,"path":"inventory/stock.go","line":39,"diffSide":"RIGHT","comments":{"nodes":[
 {"databaseId":3,"url":"https://github.com/example/shop/pull/42#discussion_r3","body":"Does wrapping the error break callers that compare with ` + "`==`" + `?","createdAt":"2026-11-19T11:00:00Z","author":{"login":"kim"},"reactionGroups":[{"content":"EYES","reactors":{"totalCount":1}}]},
 {"databaseId":4,"url":"https://github.com/example/shop/pull/42#discussion_r4","body":"No: ` + "`errors.Is`" + ` still finds it, and nothing here uses ` + "`==`" + `.","createdAt":"2026-11-19T12:00:00Z","author":{"login":"sam"},"reactionGroups":[{"content":"HEART","reactors":{"totalCount":1}}]}]}},
{"id":"T3","isResolved":true,"viewerCanUnresolve":true,"path":"inventory/stock.go","line":32,"diffSide":"RIGHT","comments":{"nodes":[
 {"databaseId":5,"url":"https://github.com/example/shop/pull/42#discussion_r5","body":"Zero should be refused too.","createdAt":"2026-11-18T09:00:00Z","author":{"login":"kim"},"reactionGroups":[]}]}},
{"id":"T4","isOutdated":true,"line":null,"originalLine":12,"path":"inventory/audit.go","comments":{"nodes":[
 {"databaseId":6,"url":"https://github.com/example/shop/pull/42#discussion_r6","body":"Should the log keep the reason as well?","createdAt":"2026-11-18T10:00:00Z","author":{"login":"kim"},"reactionGroups":[]}]}}
]}}}}}`

const shootConversation = `{"data":{"repository":{"pullRequest":{"comments":{"nodes":[
 {"url":"https://github.com/example/shop/pull/42#issuecomment-1","body":"Can this go out with Thursday's release?","createdAt":"2026-11-19T13:00:00Z","author":{"login":"alex"},"reactionGroups":[{"content":"ROCKET","reactors":{"totalCount":2}}]}]},
"reviews":{"nodes":[
 {"url":"https://github.com/example/shop/pull/42#pullrequestreview-1","body":"Two questions inline, otherwise fine.","state":"COMMENTED","submittedAt":"2026-11-18T11:00:00Z","author":{"login":"kim"},"reactionGroups":[]}]}}}}}`

package main

import (
	"io"
	"sync"
	"time"
)

const (
	// agentFrame is the shortest gap between two batches of agent output sent to the window.
	agentFrame = 33 * time.Millisecond
	// agentPending is how much unsent output may wait before reading from the agent pauses.
	agentPending = 1 << 20
)

// coalesce reads r until it fails and hands what it read to flush in batches: the first bytes
// after a quiet spell at once, then whatever arrived meanwhile at most once per every. Reading
// pauses while limit bytes wait to be flushed. Everything read is flushed before coalesce returns.
func coalesce(r io.Reader, every time.Duration, limit int, flush func([]byte)) {
	var (
		mu      sync.Mutex
		cond    = sync.NewCond(&mu)
		pending []byte
		done    bool
	)
	go func() {
		buf := make([]byte, 32<<10)
		for {
			n, err := r.Read(buf)
			mu.Lock()
			for len(pending) >= limit {
				cond.Wait()
			}
			pending = append(pending, buf[:n]...)
			done = err != nil
			cond.Broadcast()
			mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	for {
		mu.Lock()
		for len(pending) == 0 && !done {
			cond.Wait()
		}
		chunk, finished := pending, done
		pending = nil
		cond.Broadcast()
		mu.Unlock()
		if len(chunk) > 0 {
			flush(chunk)
		}
		if finished {
			return
		}
		time.Sleep(every)
	}
}

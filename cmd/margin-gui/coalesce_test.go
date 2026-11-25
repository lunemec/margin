package main

import (
	"bytes"
	"io"
	"testing"
	"time"
)

// A producer writing many small pieces reaches the window in a few batches, in order and complete.
func TestCoalesceBatchesSmallWrites(t *testing.T) {
	pr, pw := io.Pipe()
	var want bytes.Buffer
	go func() {
		for i := 0; i < 2000; i++ {
			piece := []byte{byte('a' + i%26)}
			want.Write(piece)
			pw.Write(piece)
			if i%100 == 0 {
				time.Sleep(time.Millisecond)
			}
		}
		pw.Close()
	}()
	var got bytes.Buffer
	flushes := 0
	coalesce(pr, 20*time.Millisecond, 1<<20, func(b []byte) {
		flushes++
		got.Write(b)
	})
	if !bytes.Equal(got.Bytes(), want.Bytes()) {
		t.Fatalf("got %d bytes, want %d, or out of order", got.Len(), want.Len())
	}
	if flushes > 50 {
		t.Fatalf("2000 writes took %d flushes; they should be batched", flushes)
	}
}

// After a quiet spell, output (an echoed key) is not held back.
func TestCoalesceFlushesFirstBytesAtOnce(t *testing.T) {
	pr, pw := io.Pipe()
	got := make(chan time.Time, 4)
	go coalesce(pr, time.Hour, 1<<20, func([]byte) { got <- time.Now() })
	start := time.Now()
	pw.Write([]byte("x"))
	select {
	case at := <-got:
		if d := at.Sub(start); d > 100*time.Millisecond {
			t.Fatalf("first bytes took %v", d)
		}
	case <-time.After(time.Second):
		t.Fatal("first bytes never flushed")
	}
	pw.Close()
}

// A slow window makes reading pause instead of buffering without bound.
func TestCoalescePausesReadingWhenWindowIsBehind(t *testing.T) {
	pr, pw := io.Pipe()
	release := make(chan struct{})
	go coalesce(pr, 0, 8, func([]byte) { <-release })
	pw.Write(make([]byte, 8)) // flushed, flush blocks
	pw.Write(make([]byte, 8)) // pending reaches limit
	pw.Write(make([]byte, 8)) // read, but held until there is room
	wrote := make(chan struct{})
	go func() { pw.Write(make([]byte, 8)); close(wrote) }()
	select {
	case <-wrote:
		t.Fatal("reading went on while the window was behind")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case <-wrote:
	case <-time.After(time.Second):
		t.Fatal("reading did not resume")
	}
	pw.Close()
}

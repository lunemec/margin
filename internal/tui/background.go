package tui

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
)

var oscBg = regexp.MustCompile(`\x1b\]11;rgba?:([0-9a-fA-F]+)/([0-9a-fA-F]+)/([0-9a-fA-F]+)`)

// terminalIsLight asks the terminal for its background colour (OSC 11, which tmux answers too),
// with a device attributes request behind it so a terminal that ignores OSC 11 answers at once.
// Without an answer $COLORFGBG decides, and without that the background counts as dark.
func terminalIsLight() bool {
	if light, ok := queryBackground(); ok {
		return light
	}
	if v := os.Getenv("COLORFGBG"); v != "" {
		bg := v[strings.LastIndex(v, ";")+1:]
		return bg == "7" || bg == "15"
	}
	return false
}

func queryBackground() (light, ok bool) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false, false
	}
	defer tty.Close()
	old, err := term.MakeRaw(tty.Fd())
	if err != nil {
		return false, false
	}
	defer term.Restore(tty.Fd(), old)
	if _, err := tty.WriteString("\x1b]11;?\x1b\\\x1b[c"); err != nil {
		return false, false
	}
	deadline := time.Now().Add(400 * time.Millisecond)
	var got []byte
	buf := make([]byte, 256)
	for time.Now().Before(deadline) {
		if tty.SetReadDeadline(deadline) != nil {
			return false, false
		}
		n, err := tty.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil {
			break
		}
		if i := strings.Index(string(got), "\x1b[?"); i >= 0 && strings.ContainsRune(string(got[i:]), 'c') {
			break
		}
	}
	m := oscBg.FindSubmatch(got)
	if m == nil {
		return false, false
	}
	var lum float64
	for i, w := range []float64{0.299, 0.587, 0.114} {
		hex := string(m[i+1])
		v, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return false, false
		}
		lum += w * float64(v) / float64(uint64(1)<<(4*len(hex))-1)
	}
	return lum > 0.5, true
}

package vt10x

import (
	"fmt"
	"strings"
	"testing"
)

// frame approximates one Ink repaint of Claude's prompt area: erase N lines
// upward, then redraw truecolor text, a full-width rule, the prompt and a
// status line.
func frame(i int) []byte {
	var b strings.Builder
	for j := 0; j < 6; j++ {
		b.WriteString("\x1b[2K\x1b[1A")
	}
	b.WriteString("\x1b[2K\x1b[G")
	fmt.Fprintf(&b, "\x1b[38;2;215;119;87m✻\x1b[39m \x1b[38;2;153;153;153mThinking… (%ds · esc to interrupt)\x1b[39m\r\n", i)
	b.WriteString("\x1b[38;2;136;136;136m" + strings.Repeat("─", 160) + "\x1b[39m\r\n")
	b.WriteString("❯ \x1b[7m \x1b[27m\r\n")
	b.WriteString("\x1b[38;2;136;136;136m" + strings.Repeat("─", 160) + "\x1b[39m\r\n")
	b.WriteString("  \x1b[38;2;175;135;255m⏵⏵ bypass permissions on\x1b[39m · 日本語 👩‍💻\r\n")
	return []byte(b.String())
}

var frames = func() [][]byte {
	out := make([][]byte, 64)
	for i := range out {
		out[i] = frame(i)
	}
	return out
}()

func benchBytes() int64 {
	n := 0
	for _, f := range frames {
		n += len(f)
	}
	return int64(n)
}

func BenchmarkRepaint(b *testing.B) {
	t := New(WithSize(160, 48))
	b.SetBytes(benchBytes())
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, f := range frames {
			t.Write(f)
		}
	}
}

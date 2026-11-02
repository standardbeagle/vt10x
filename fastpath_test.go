package vt10x

import (
	"io"
	"log"
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"
)

// decodeShortRune agrees with utf8.DecodeRune on every two- and three-byte
// input it accepts, and accepts every valid one.
func TestDecodeShortRuneMatchesStdlib(t *testing.T) {
	for b0 := 0; b0 < 256; b0++ {
		for b1 := 0; b1 < 256; b1++ {
			for _, b2 := range []byte{0x00, 0x7F, 0x80, 0x9F, 0xA0, 0xBF, 0xC0, 0xFF} {
				p := []byte{byte(b0), byte(b1), b2}
				r, size := decodeShortRune(p)
				want, wsize := utf8.DecodeRune(p)
				valid := !(want == utf8.RuneError && wsize == 1) && (wsize == 2 || wsize == 3)
				if size == 0 {
					if valid {
						t.Fatalf("% x: rejected a valid %d-byte rune", p, wsize)
					}
					continue
				}
				if !valid || r != want || size != wsize {
					t.Fatalf("% x: got %U/%d, stdlib %U/%d", p, r, size, want, wsize)
				}
			}
		}
	}
}

// write's plain-text and CSI-parameter fast paths must leave the screen
// exactly as the per-rune path does. A DebugLogger forces every byte down that
// path, so a logging terminal is the reference.
func TestWriteFastPathsMatchPerRunePath(t *testing.T) {
	pieces := []string{
		"hello ", "x", " ", "\r", "\n", "\r\n", "\t", "\b", "\x7f",
		"\x1b[31m", "\x1b[0m", "\x1b[38;2;215;119;87m", "\x1b[2K", "\x1b[1A", "\x1b[G", "\x1b[5;10H",
		"\x1b[?25l", "\x1b[?1049h", "\x1b[?1049l", "\x1b[>1u", "\x1b[3 q", "\x1b[1;5r", "\x1b[r", "\x1b[4h", "\x1b[4l",
		"\x1b[" + strings.Repeat("1;", 140) + "m", "\x1b[" + strings.Repeat("9", 300),
		"\x1b]0;title\a", "\x1b]8;;http://x\x1b\\", "\x1b(0", "lqk", "\x1b(B", "\x1bc", "\x1b7", "\x1b8", "\x1b#8",
		"─", "日本語", "👩‍💻", "é", "é", "؀x", "\xe2\x94", "\x80", "\xff",
		"\xed\xa0\x80", "\xe0\x80\xaf", "\xe0\xa0\x80", "\xc0\xaf", "\xc1\xbf", "\xc2\x85", "\xc2", "\xef\xbf\xbf", "\xf0\x9f\x98\x80", "\xed\x9f\xbf",
	}
	rng := rand.New(rand.NewSource(5))
	for iter := 0; iter < 300; iter++ {
		var b strings.Builder
		for b.Len() < 4000 {
			b.WriteString(pieces[rng.Intn(len(pieces))])
		}
		data := []byte(b.String())

		fast := New(WithSize(40, 12)).(*terminal)
		slow := New(WithSize(40, 12)).(*terminal)
		slow.DebugLogger = log.New(io.Discard, "", 0)
		for rest := data; len(rest) > 0; {
			n := 1 + rng.Intn(min(len(rest), 300))
			fast.Write(rest[:n])
			slow.Write(rest[:n])
			rest = rest[n:]
		}
		if fast.Cursor() != slow.Cursor() || fast.Mode() != slow.Mode() || fast.Title() != slow.Title() {
			t.Fatalf("iter %d: cursor/mode/title differ: %+v %v vs %+v %v", iter, fast.Cursor(), fast.Mode(), slow.Cursor(), slow.Mode())
		}
		for y := 0; y < 12; y++ {
			for x := 0; x < 40; x++ {
				if f, s := fast.Cell(x, y), slow.Cell(x, y); f != s {
					t.Fatalf("iter %d cell %d,%d: %+v vs %+v", iter, x, y, f, s)
				}
			}
		}
	}
}

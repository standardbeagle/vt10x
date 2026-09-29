package vt10x

import (
	"strings"
	"testing"
)

// A CSI with a private prefix (< = > ?) or an intermediate byte (0x20-0x2F) is
// a different sequence from the plain one that shares its final byte. The
// kitty keyboard protocol alone sends CSI > 1 u, CSI < u, CSI ? u, CSI = 1;1 u
// -- none of which is DECRC.
func TestCSIPrefixedSequencesAreNotTheUnprefixedOnes(t *testing.T) {
	ignored := []string{
		"\x1b[>1u", "\x1b[<u", "\x1b[?u", "\x1b[=1;1u", // kitty keyboard
		"\x1b[>4;2m",         // xterm modifyOtherKeys, not SGR
		"\x1b[>c", "\x1b[=c", // DA2 / DA3
		"\x1b[>0q",                   // XTVERSION
		"\x1b[?2026h", "\x1b[?2026l", // synchronized update
		"\x1b[>1;2H", "\x1b[?3;3H", // not CUP
		"\x1b[>3A", "\x1b[=3B", "\x1b[<3C", "\x1b[>3D", // not cursor moves
		"\x1b[>3J", "\x1b[=2K", "\x1b[>1s", "\x1b[>5n",
		"\x1b[?1s", "\x1b[?5;5r",
		"\x1b[2 q", "\x1b[0 q", "\x1b[1;2 q", // DECSCUSR and other SP-intermediate
		"\x1b[!p", "\x1b[\"q", "\x1b[$p", "\x1b[?2026$p", "\x1b[3\"p", // DECSTR, DECSCA, DECRQM
		"\x1b[ 1q", "\x1b[1 2q", // parameter byte after an intermediate: invalid, ignored whole
		"\x1b[1>u", "\x1b[1;>u", // prefix byte after a parameter: invalid
	}
	for _, seq := range ignored {
		term := New(WithSize(20, 5))
		feed(t, term, "\x1b[31mab\x1b[3;4H")
		before := cursorAt(term)
		modeBefore := term.Mode()
		feed(t, term, seq)
		if got := cursorAt(term); got != before {
			t.Errorf("%q moved the cursor %v -> %v", seq, before, got)
		}
		if term.Mode() != modeBefore {
			t.Errorf("%q changed modes %b -> %b", seq, modeBefore, term.Mode())
		}
		feed(t, term, "X")
		g := term.Cell(before[0], before[1])
		if g.Char != 'X' || g.FG != Red || g.Mode != 0 {
			t.Errorf("%q disturbed the pen: cell %+v", seq, g)
		}
		if got := rowText(term, 0); !strings.HasPrefix(got, "ab") {
			t.Errorf("%q disturbed row 0: %q", seq, got)
		}
	}
}

func TestKittyPushDoesNotRestoreCursor(t *testing.T) {
	term := New(WithSize(20, 5))
	feed(t, term, "ab\x1b[>1ucd")
	if got, want := cursorAt(term), [2]int{4, 0}; got != want {
		t.Fatalf("cursor %v, want %v", got, want)
	}
	if got := rowText(term, 0)[:4]; got != "abcd" {
		t.Fatalf("row %q, want abcd", got)
	}
}

// The unprefixed forms keep working.
func TestCSIUnprefixedStillDispatches(t *testing.T) {
	term := New(WithSize(20, 5))
	feed(t, term, "ab\x1b[sxx\x1b[uc")
	if got := rowText(term, 0)[:4]; got != "abcx" {
		t.Fatalf("save/restore: %q", got)
	}
	feed(t, term, "\x1b[?25l")
	if term.CursorVisible() {
		t.Fatal("?25l did not hide the cursor")
	}
	feed(t, term, "\x1b[?25h")
	if !term.CursorVisible() {
		t.Fatal("?25h did not show the cursor")
	}
	feed(t, term, "\x1b[2;3H")
	if got, want := cursorAt(term), [2]int{2, 1}; got != want {
		t.Fatalf("CUP: %v want %v", got, want)
	}
	feed(t, term, "\x1b[4mU\x1b[m")
	if term.Cell(2, 1).Mode&attrUnderline == 0 {
		t.Fatal("SGR 4 lost")
	}
}

// Cursor-up/down honour the scroll region (xterm): inside the region they
// stop at its margins, outside it at the screen edge.
func TestCursorVerticalMovesStopAtScrollMargins(t *testing.T) {
	const region = "\x1b[2;5r" // rows 2..5 -> y 1..4
	cases := []struct {
		name string
		in   string
		want [2]int
	}{
		{"CUD inside", region + "\x1b[3;1H\x1b[50B", [2]int{0, 4}},
		{"CUU inside", region + "\x1b[3;1H\x1b[50A", [2]int{0, 1}},
		{"CNL inside", region + "\x1b[3;4H\x1b[50E", [2]int{0, 4}},
		{"CPL inside", region + "\x1b[3;4H\x1b[50F", [2]int{0, 1}},
		{"VPR inside", region + "\x1b[3;1H\x1b[50e", [2]int{0, 4}},
		{"CUD below region", region + "\x1b[8;3H\x1b[50B", [2]int{2, 9}},
		{"CUD above region", region + "\x1b[1;3H\x1b[50B", [2]int{2, 4}},
		{"CUU above region", region + "\x1b[1;3H\x1b[50A", [2]int{2, 0}},
		{"CUU below region stops at top margin", region + "\x1b[8;3H\x1b[50A", [2]int{2, 1}},
		{"CNL below region", region + "\x1b[8;3H\x1b[50E", [2]int{0, 9}},
		{"CUD short", region + "\x1b[3;1H\x1b[1B", [2]int{0, 3}},
		{"CUD zero means one", region + "\x1b[3;1H\x1b[0B", [2]int{0, 3}},
		{"CNL zero means one", region + "\x1b[3;5H\x1b[0E", [2]int{0, 3}},
		{"CUD no region", "\x1b[3;1H\x1b[50B", [2]int{0, 9}},
		{"CUU no region", "\x1b[3;1H\x1b[50A", [2]int{0, 0}},
	}
	for _, c := range cases {
		term := New(WithSize(20, 10))
		feed(t, term, c.in)
		if got := cursorAt(term); got != c.want {
			t.Errorf("%s: %q -> %v, want %v", c.name, c.in, got, c.want)
		}
	}
}

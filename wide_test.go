package vt10x

import (
	"strings"
	"testing"
)

// cellDump renders row y one cell per element: the cluster text, "<c>" for a
// wide continuation, and a trailing "!" on a wide first cell. Trailing blanks
// are dropped.
func cellDump(term Terminal, y int) string {
	cols, _ := term.Size()
	var parts []string
	for x := 0; x < cols; x++ {
		g := term.Cell(x, y)
		switch {
		case g.IsWideContinuation():
			parts = append(parts, "<c>")
		case g.IsWide():
			parts = append(parts, g.Text()+"!")
		default:
			parts = append(parts, g.Text())
		}
	}
	for len(parts) > 0 && parts[len(parts)-1] == " " {
		parts = parts[:len(parts)-1]
	}
	return strings.Join(parts, "|")
}

func TestWideCharacterOccupiesTwoCells(t *testing.T) {
	term := New(WithSize(10, 3))
	feed(t, term, "a世b")
	if got, want := cursorAt(term), [2]int{4, 0}; got != want {
		t.Fatalf("cursor %v want %v", got, want)
	}
	if got, want := cellDump(term, 0), "a|世!|<c>|b"; got != want {
		t.Fatalf("cells %q want %q", got, want)
	}
	g, c := term.Cell(1, 0), term.Cell(2, 0)
	if g.Char != '世' || !g.IsWide() || g.IsWideContinuation() || g.Text() != "世" {
		t.Errorf("head %+v", g)
	}
	if c.Char != 0 || c.IsWide() || !c.IsWideContinuation() || c.Text() != "" {
		t.Errorf("continuation %+v", c)
	}
	if term.Cell(0, 0).IsWide() || term.Cell(3, 0).IsWideContinuation() {
		t.Error("narrow cells flagged wide")
	}
	if got := term.String()[:len("a世b")]; got != "a世b" {
		t.Errorf("String() %q", got)
	}
}

func TestColoursCoverBothHalvesOfAWideCell(t *testing.T) {
	term := New(WithSize(10, 3))
	feed(t, term, "\x1b[38;2;1;2;3;48;5;9m世")
	h, c := term.Cell(0, 0), term.Cell(1, 0)
	if h.FG != RGB(1, 2, 3) || c.FG != h.FG || h.BG != 9 || c.BG != 9 {
		t.Errorf("head %+v continuation %+v", h, c)
	}
}

func TestClustersShareOneCell(t *testing.T) {
	cases := []struct {
		name, in string
		cursor   [2]int
		cells    string
	}{
		{"zwj sequence", "👩‍💻x", [2]int{3, 0}, "👩‍💻!|<c>|x"},
		{"combining mark", "éx", [2]int{2, 0}, "é|x"},
		{"two marks", "ẹ́x", [2]int{2, 0}, "ẹ́|x"},
		{"emoji", "😀x", [2]int{3, 0}, "😀!|<c>|x"},
		{"skin tone", "👍🏽x", [2]int{3, 0}, "👍🏽!|<c>|x"},
		{"vs16 widens", "❤️x", [2]int{3, 0}, "❤️!|<c>|x"},
		{"flag", "🇯🇵x", [2]int{3, 0}, "🇯🇵!|<c>|x"},
		{"two flags", "🇯🇵🇩🇪x", [2]int{5, 0}, "🇯🇵!|<c>|🇩🇪!|<c>|x"},
		{"cjk run", "日本語", [2]int{6, 0}, "日!|<c>|本!|<c>|語!|<c>"},
		{"fullwidth", "Ａb", [2]int{3, 0}, "Ａ!|<c>|b"},
		{"korean jamo", "한x", [2]int{3, 0}, "한!|<c>|x"},
	}
	for _, c := range cases {
		term := New(WithSize(12, 3))
		feed(t, term, c.in)
		if got := cursorAt(term); got != c.cursor {
			t.Errorf("%s: cursor %v want %v", c.name, got, c.cursor)
		}
		if got := cellDump(term, 0); got != c.cells {
			t.Errorf("%s: cells %q want %q", c.name, got, c.cells)
		}
	}
}

func TestClusterGlyphAPI(t *testing.T) {
	term := New(WithSize(10, 2))
	feed(t, term, "👩‍💻")
	g := term.Cell(0, 0)
	if g.Char != '👩' || g.Extra != "‍💻" || g.Text() != "👩‍💻" || !g.IsWide() {
		t.Fatalf("%+v", g)
	}
}

// A cluster continues across Write calls, including a split in the middle of a
// UTF-8 sequence: a PTY read boundary falls anywhere.
func TestClustersSurviveWriteBoundaries(t *testing.T) {
	streams := []string{
		"a世b",
		"👩‍💻x",
		"ẹ́x",
		"🇯🇵🇩🇪x",
		"👍🏽 ❤️日本語 \x1b[31mé\x1b[0m",
		"\x1b[2Kab\x1b[1;5Hé́",
	}
	for _, s := range streams {
		whole := New(WithSize(12, 3))
		feed(t, whole, s)
		for cut := 1; cut < len(s); cut++ {
			split := New(WithSize(12, 3))
			feed(t, split, s[:cut])
			feed(t, split, s[cut:])
			for y := 0; y < 3; y++ {
				if a, b := cellDump(whole, y), cellDump(split, y); a != b {
					t.Fatalf("%q split at byte %d: row %d %q vs %q", s, cut, y, b, a)
				}
			}
			if cursorAt(whole) != cursorAt(split) {
				t.Fatalf("%q split at byte %d: cursor %v vs %v", s, cut, cursorAt(split), cursorAt(whole))
			}
		}
		// And one byte at a time.
		drip := New(WithSize(12, 3))
		for i := 0; i < len(s); i++ {
			feed(t, drip, s[i:i+1])
		}
		for y := 0; y < 3; y++ {
			if a, b := cellDump(whole, y), cellDump(drip, y); a != b {
				t.Fatalf("%q dripped: row %d %q vs %q", s, y, b, a)
			}
		}
	}
}

func TestWriteReportsEveryByteConsumed(t *testing.T) {
	term := New(WithSize(10, 2))
	for _, p := range []string{"世"[:2], "世"[2:], "abc", "\xf0\x9f", "\x98\x80"} {
		n, err := term.Write([]byte(p))
		if n != len(p) || err != nil {
			t.Fatalf("Write(%q) = %d, %v", p, n, err)
		}
	}
	if got := cellDump(term, 0); got != "世!|<c>|a|b|c|😀!|<c>" {
		t.Fatalf("%q", got)
	}
}

func TestInvalidUTF8IsSkippedNotFatal(t *testing.T) {
	term := New(WithSize(10, 2))
	feed(t, term, "a\xffb\xe4Ac")
	if got := cellDump(term, 0); got != "a|b|A|c" {
		t.Fatalf("%q", got)
	}
}

func TestWideCharacterAtLastColumnWrapsFirst(t *testing.T) {
	term := New(WithSize(5, 3))
	feed(t, term, "abcd世e")
	if got, want := cellDump(term, 0), "a|b|c|d"; got != want {
		t.Errorf("row0 %q want %q", got, want)
	}
	if got, want := cellDump(term, 1), "世!|<c>|e"; got != want {
		t.Errorf("row1 %q want %q", got, want)
	}
	if got, want := cursorAt(term), [2]int{3, 1}; got != want {
		t.Errorf("cursor %v want %v", got, want)
	}

	// Fits exactly: no wrap until the next character.
	term = New(WithSize(5, 3))
	feed(t, term, "abc世")
	if got, want := cellDump(term, 0), "a|b|c|世!|<c>"; got != want {
		t.Errorf("exact fit %q want %q", got, want)
	}
	feed(t, term, "x")
	if got, want := cellDump(term, 1), "x"; got != want {
		t.Errorf("after exact fit row1 %q want %q", got, want)
	}

	// Without autowrap the character has nowhere to go: the last cell stays
	// blank and the next character lands there.
	term = New(WithSize(5, 3))
	feed(t, term, "\x1b[?7labcd世e")
	if got, want := cellDump(term, 0), "a|b|c|d|e"; got != want {
		t.Errorf("no-wrap row0 %q want %q", got, want)
	}
	if got := cellDump(term, 1); got != "" {
		t.Errorf("no-wrap row1 %q", got)
	}
}

func TestOverwritingEitherHalfClearsBoth(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"over head", "世世\r" + "a", "a| |世!|<c>"},
		{"over continuation", "世世\x1b[2Gb", " |b|世!|<c>"},
		{"wide over two halves", "世世\x1b[2G日", " |日!|<c>| "},
		{"wide over head", "世a\r日", "日!|<c>|a"},
		{"narrow after wide cluster start", "a世\x1b[3Gb", "a| |b"},
	}
	for _, c := range cases {
		term := New(WithSize(8, 2))
		feed(t, term, c.in)
		got := strings.TrimRight(cellDump(term, 0), "| ")
		want := strings.TrimRight(c.want, "| ")
		if got != want {
			t.Errorf("%s: %q want %q", c.name, got, want)
		}
	}
}

func TestErasingHalfAWideCellClearsTheOther(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"EL0 from continuation", "世世\x1b[2G\x1b[K", ""},
		{"EL0 from head", "a世\x1b[2G\x1b[K", "a"},
		{"EL1 through head", "世世\x1b[1G\x1b[1K", " | |世!|<c>"},
		{"EL1 through continuation", "世世\x1b[2G\x1b[1K", " | |世!|<c>"},
		{"EL1 ends on head", "世世\x1b[3G\x1b[1K", ""}, // through the second head: its continuation goes too
		{"ECH on head", "世a\x1b[1G\x1b[X", " | |a"},
		{"ECH on continuation", "世a\x1b[2G\x1b[X", " | |a"},
		{"ECH range ends on head", "a世世\x1b[1G\x1b[4X", ""},
		{"ED0", "a世\x1b[3G\x1b[J", "a| "},
	}
	for _, c := range cases {
		term := New(WithSize(8, 3))
		feed(t, term, c.in)
		got := strings.TrimRight(cellDump(term, 0), "| ")
		want := strings.TrimRight(c.want, "| ")
		if got != want {
			t.Errorf("%s: %q want %q", c.name, got, want)
		}
		for x := 0; x < 8; x++ {
			g := term.Cell(x, 0)
			if g.IsWideContinuation() && (x == 0 || !term.Cell(x-1, 0).IsWide()) {
				t.Errorf("%s: orphan continuation at %d", c.name, x)
			}
			if g.IsWide() && (x == 7 || !term.Cell(x+1, 0).IsWideContinuation()) {
				t.Errorf("%s: orphan head at %d", c.name, x)
			}
		}
	}
}

func TestInsertAndDeleteCharsNeverLeaveHalves(t *testing.T) {
	cases := []struct{ name, in string }{
		{"ICH at continuation", "世a\x1b[2G\x1b[@"},
		{"ICH at head", "世a\x1b[1G\x1b[@"},
		{"ICH pushes head off the end", "abc世\x1b[1G\x1b[@"},
		{"ICH pushes head to last col", "abc世\x1b[1G\x1b[@x"},
		{"DCH at continuation", "世a\x1b[2G\x1b[P"},
		{"DCH at head", "世a\x1b[1G\x1b[P"},
		{"DCH ends on continuation", "a世b\x1b[1G\x1b[2P"},
		{"DCH big", "a世世b\x1b[2G\x1b[3P"},
		{"ICH big", "a世世b\x1b[2G\x1b[3@"},
		{"scroll keeps pairs", "世\x1b[S"},
	}
	for _, c := range cases {
		term := New(WithSize(5, 3))
		feed(t, term, c.in)
		for y := 0; y < 3; y++ {
			for x := 0; x < 5; x++ {
				g := term.Cell(x, y)
				if g.IsWideContinuation() && (x == 0 || !term.Cell(x-1, y).IsWide()) {
					t.Errorf("%s: orphan continuation at %d,%d: %q", c.name, x, y, cellDump(term, y))
				}
				if g.IsWide() && (x == 4 || !term.Cell(x+1, y).IsWideContinuation()) {
					t.Errorf("%s: orphan head at %d,%d: %q", c.name, x, y, cellDump(term, y))
				}
			}
		}
	}

	// Concrete results.
	check := func(in, want string) {
		t.Helper()
		term := New(WithSize(5, 3))
		feed(t, term, in)
		if got := strings.TrimRight(cellDump(term, 0), "| "); got != want {
			t.Errorf("%q -> %q want %q", in, got, want)
		}
	}
	check("世a\x1b[2G\x1b[@", " | | |a")   // continuation hit: head cleared too, blank inserted
	check("世a\x1b[1G\x1b[P", " |a")       // head deleted: its continuation cannot stay
	check("世a\x1b[2G\x1b[P", " |a")       // continuation deleted: head cleared
	check("abc世\x1b[1G\x1b[@", " |a|b|c") // head pushed to last col loses its continuation
	check("a世b\x1b[1G\x1b[2P", " |b")     // deletes a and 世's head; continuation blanked
}

func TestResizeNeverLeavesAHalfAtTheEdge(t *testing.T) {
	term := New(WithSize(6, 2))
	feed(t, term, "abcd世")
	term.Resize(5, 2)
	if got := cellDump(term, 0); got != "a|b|c|d" {
		t.Fatalf("shrunk %q", got)
	}
	term.Resize(8, 2)
	feed(t, term, "\r\x1b[5G世")
	if got := cellDump(term, 0); got != "a|b|c|d|世!|<c>" {
		t.Fatalf("grown %q", got)
	}
}

func TestClusterDoesNotJoinAcrossControlSequences(t *testing.T) {
	// A mark after a cursor movement has no base to attach to.
	term := New(WithSize(8, 2))
	feed(t, term, "e\x1b[Ćx")
	if got := cellDump(term, 0); strings.Contains(got, "é") {
		t.Fatalf("mark joined across CSI: %q", got)
	}
	term = New(WithSize(8, 2))
	feed(t, term, "e\ŕx")
	if got := cellDump(term, 0); strings.Contains(got, "é") {
		t.Fatalf("mark joined across CR: %q", got)
	}
}

func TestPrintableASCIIStaysNarrowAndSeparate(t *testing.T) {
	term := New(WithSize(100, 2))
	all := ""
	for r := rune(0x20); r < 0x7F; r++ {
		all += string(r)
	}
	feed(t, term, all)
	for i, r := range all {
		g := term.Cell(i, 0)
		if g.Char != r || g.Extra != "" || g.IsWide() || g.IsWideContinuation() {
			t.Fatalf("%q at %d: %+v", r, i, g)
		}
	}
	if got, want := cursorAt(term), [2]int{len(all), 0}; got != want {
		t.Fatalf("cursor %v want %v", got, want)
	}
}

// Turning autowrap on after characters were written at the right margin with it
// off must not wrap the next character: there was never a pending wrap.
func TestAutowrapOffLeavesNoPendingWrap(t *testing.T) {
	term := New(WithSize(5, 3))
	feed(t, term, "\x1b[?7l"+strings.Repeat("x", 8)+"\x1b[?7hyyy")
	if got := cellDump(term, 0); got != "x|x|x|x|y" {
		t.Errorf("row0 %q", got)
	}
	if got := cellDump(term, 1); got != "y|y" {
		t.Errorf("row1 %q", got)
	}
	if got, want := cursorAt(term), [2]int{2, 1}; got != want {
		t.Errorf("cursor %v want %v", got, want)
	}
}

func TestLoneSkinToneModifierIsAWideEmoji(t *testing.T) {
	term := New(WithSize(6, 2))
	feed(t, term, "🏽x")
	if got := cellDump(term, 0); got != "🏽!|<c>|x" {
		t.Fatalf("%q", got)
	}
}

// ED 1 erases from the top of the screen through the cursor, including the row
// directly above a cursor on row 1.
func TestEraseAboveClearsTheRowAboveRowOne(t *testing.T) {
	term := New(WithSize(5, 3))
	feed(t, term, "aaaaa\r\nbbbbb\r\nccccc\x1b[2;3H\x1b[1J")
	for y, want := range []string{"", "   bb", "ccccc"} {
		if got := strings.TrimRight(rowText(term, y), " "); got != strings.TrimRight(want, " ") {
			t.Errorf("row %d %q want %q", y, got, want)
		}
	}
}

func TestWriteWithAutowrapOffConsumesAPendingWrap(t *testing.T) {
	term := New(WithSize(3, 2))
	feed(t, term, "abc\x1b[?7lx\x1b[?7hyz")
	if got := cellDump(term, 0); got != "a|b|y" {
		t.Errorf("row0 %q", got)
	}
	if got := cellDump(term, 1); got != "z" {
		t.Errorf("row1 %q", got)
	}
}

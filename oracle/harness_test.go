package oracle

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	cvt "github.com/charmbracelet/x/vt"
	"github.com/standardbeagle/vt10x"
)

// snapshot is what the two emulators are compared on: the cursor, and for each
// cell its cluster text ("" for the second half of a wide character) and
// whether it is the head of a wide character.
type snapshot struct {
	cols, rows int
	cursor     [2]int
	text       [][]string
	wide       [][]bool
}

func (s snapshot) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "cursor=(%d,%d)\n", s.cursor[0], s.cursor[1])
	for y := 0; y < s.rows; y++ {
		last := s.cols - 1
		for last >= 0 && (s.text[y][last] == " ") {
			last--
		}
		var parts []string
		for x := 0; x <= last; x++ {
			switch {
			case s.text[y][x] == "" && x > 0 && s.wide[y][x-1]:
				parts = append(parts, "<c>")
			case s.wide[y][x]:
				parts = append(parts, s.text[y][x]+"!")
			default:
				parts = append(parts, s.text[y][x])
			}
		}
		fmt.Fprintf(&b, "%2d|%s\n", y, strings.Join(parts, "|"))
	}
	return b.String()
}

// diff returns "" when a and b agree, else a description of the first mismatch.
func (a snapshot) diff(b snapshot) string {
	if a.cursor != b.cursor {
		return fmt.Sprintf("cursor %v vs %v", a.cursor, b.cursor)
	}
	for y := 0; y < a.rows; y++ {
		for x := 0; x < a.cols; x++ {
			if a.text[y][x] != b.text[y][x] || a.wide[y][x] != b.wide[y][x] {
				return fmt.Sprintf("cell (%d,%d): %q wide=%v vs %q wide=%v", x, y, a.text[y][x], a.wide[y][x], b.text[y][x], b.wide[y][x])
			}
		}
	}
	return ""
}

func newSnapshot(cols, rows int) snapshot {
	s := snapshot{cols: cols, rows: rows}
	s.text = make([][]string, rows)
	s.wide = make([][]bool, rows)
	for y := range s.text {
		s.text[y] = make([]string, cols)
		s.wide[y] = make([]bool, cols)
	}
	return s
}

// vt10xWrapNext is the pending-wrap flag in vt10x's Cursor.State.
const vt10xWrapNext = 2

// hazards are situations in a stream where charmbracelet/x/vt is known to
// differ from xterm (see exempt_test.go). A stream that hits one is not a valid
// differential case, so the fuzzer skips it rather than compare.
type hazards struct {
	// wideAtEdge: a non-ASCII rune was written with the cursor in the last
	// column and no wrap pending, so a wide character has no room and a cluster
	// cannot grow to two columns. x/vt drops the character; xterm and tmux wrap
	// it (or, with autowrap off, leave the column blank).
	wideAtEdge bool
	// exactFill: a wide character ended exactly at the right margin. x/vt does
	// not set pending wrap there, so the next character clobbers its
	// continuation cell and destroys the character.
	exactFill bool
	// shiftWide: ICH or DCH ran on a row that holds a wide character. x/vt
	// shifts the two halves independently and destroys the pair; xterm and tmux
	// move it intact.
	shiftWide bool
	// cbtPending: CBT, CHT, HT, DECRC or an erase/insert/delete ran with a wrap
	// pending. x/vt keeps the pending wrap after a tab or restore (xterm and
	// tmux clear it when the cursor moves) and drops it after an erase; whether
	// xterm resets it there is not settled (tmux does not), so those are not
	// compared.
	cbtPending bool
	// outsideRegion: CUU/CUD/CNL/CPL with the cursor outside the scroll region,
	// or ICH/DCH with a region set and the cursor outside it. x/vt differs from
	// xterm on the first and ignores the second.
	outsideRegion bool
	// loneMark: a zero-width mark with no text before it to attach to. x/vt
	// gives it a cell; xterm and vt10x drop it.
	loneMark bool
	// ed1: ED 1 (CSI 1 J). x/vt erased more than through the cursor on a row
	// holding wide characters; tmux and vt10x erase exactly through it.
	ed1 bool
}

func (h hazards) any() bool {
	return h.wideAtEdge || h.exactFill || h.shiftWide || h.cbtPending || h.outsideRegion || h.loneMark || h.ed1
}

func isText(tok string) bool {
	if tok == "" {
		return false
	}
	c := tok[0]
	return c >= 0x20 && c != 0x1b && c != 0x7f
}

// startsWideCluster reports whether r may begin a two-column cluster: CJK and
// fullwidth forms, Hangul jamo, emoji and regional indicators, and the symbol
// block (0x2600-0x27BF) whose members widen to two columns with a VS16. Such a
// symbol at the last column cannot widen.
func startsWideCluster(r rune) bool {
	return r >= 0x2E80 || (r >= 0x1100 && r <= 0x11FF) || r >= 0x1F000 || (r >= 0x2600 && r <= 0x27BF)
}

func isMarkRune(r rune) bool {
	return r == 0x200d || r == 0xfe0f || r == 0xfe0e || r == 0x200b || (r >= 0x300 && r <= 0x36f)
}

var regionRE = regexp.MustCompile(`^\x1b\[(\d*);?(\d*)r`)

// runVT10x feeds the tokens to vt10x and reports the hazards it met on the way.
// Escape sequences go in one Write each; text is written a rune at a time, so
// every cluster is completed across Write boundaries.
func runVT10x(cols, rows int, tokens []string) (snap snapshot, h hazards) {
	term := vt10x.New(vt10x.WithSize(cols, rows))
	rowHasWide := func(y int) bool {
		for x := 0; x < cols; x++ {
			if term.Cell(x, y).IsWide() {
				return true
			}
		}
		return false
	}
	write := func(b string) {
		if _, err := term.Write([]byte(b)); err != nil {
			panic(err)
		}
	}
	top, bottom := 0, rows-1
	prev := ""
	for _, tok := range tokens {
		c := term.Cursor()
		if strings.HasPrefix(tok, "\x1b[") {
			last := strings.TrimRight(tok, "\r")[len(strings.TrimRight(tok, "\r"))-1]
			pending := c.State&vt10xWrapNext != 0
			inRegion := c.Y >= top && c.Y <= bottom
			switch last {
			case 'Z', 'I', 'X', 'K':
				h.cbtPending = h.cbtPending || pending
			case '@', 'P':
				h.cbtPending = h.cbtPending || pending
				if rowHasWide(c.Y) {
					h.shiftWide = true
				}
				if (top != 0 || bottom != rows-1) && !inRegion {
					h.outsideRegion = true
				}
			case 'L', 'M':
				// x/vt moves the cursor row on IL/DL inside a sub-region.
				if top != 0 || bottom != rows-1 {
					h.outsideRegion = true
				}
			case 'A', 'B', 'E', 'F', 'e':
				if !inRegion {
					h.outsideRegion = true
				}
			case 'J':
				h.cbtPending = h.cbtPending || pending
				if strings.Contains(tok, "1J") {
					h.ed1 = true
				}
			}
			if m := regionRE.FindStringSubmatch(tok); m != nil {
				top, bottom = 0, rows-1
				if m[1] != "" || m[2] != "" {
					t, _ := strconv.Atoi(m[1])
					b, _ := strconv.Atoi(m[2])
					top, bottom = t-1, b-1
				}
			}
			write(tok)
		} else {
			if pending := c.State&vt10xWrapNext != 0; pending && (strings.HasPrefix(tok, "\t") || tok == "\x1b8") {
				h.cbtPending = true
			}
			for _, r := range tok {
				c = term.Cursor()
				pending := c.State&vt10xWrapNext != 0
				if c.X == cols-1 && !pending && startsWideCluster(r) {
					h.wideAtEdge = true
				}
				if isMarkRune(r) && !isText(prev) {
					h.loneMark = true
				}
				write(string(r))
				c = term.Cursor()
				if c.X == cols-1 && c.State&vt10xWrapNext != 0 && term.Cell(cols-1, c.Y).IsWideContinuation() {
					h.exactFill = true
				}
			}
		}
		prev = tok
	}
	snap = newSnapshot(cols, rows)
	c := term.Cursor()
	snap.cursor = [2]int{c.X, c.Y}
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			g := term.Cell(x, y)
			snap.text[y][x] = g.Text()
			snap.wide[y][x] = g.IsWide()
			if g.IsWideContinuation() {
				snap.text[y][x] = ""
			}
		}
	}
	return snap, h
}

func hasNonASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return true
		}
	}
	return false
}

// runXVT feeds the whole stream to charmbracelet/x/vt in a single Write, which
// is how x/vt resolves clusters (it flushes at the end of each Write).
func runXVT(cols, rows int, stream string) snapshot {
	emu := cvt.NewEmulator(cols, rows)
	defer emu.Close()
	// x/vt answers queries on a pipe; without a reader Write blocks.
	go io.Copy(io.Discard, emu)
	if _, err := emu.Write([]byte(stream)); err != nil {
		panic(err)
	}
	snap := newSnapshot(cols, rows)
	p := emu.CursorPosition()
	snap.cursor = [2]int{p.X, p.Y}
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			c := emu.CellAt(x, y)
			if c == nil {
				continue
			}
			snap.text[y][x] = c.Content
			snap.wide[y][x] = c.Width > 1
			// x/vt marks the second half of a wide character with an empty
			// cell, and sometimes leaves one behind with no wide character
			// before it (after an overwrite or a shift). Such an orphan
			// renders as a blank, which is what vt10x stores.
			if c.Content == "" && (x == 0 || !snap.wide[y][x-1]) {
				snap.text[y][x] = " "
			}
		}
	}
	return snap
}

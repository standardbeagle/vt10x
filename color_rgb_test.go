package vt10x

import "testing"

// Upstream packed RGB untagged into the same uint32 space as the palette
// indices, so every rgb(0, 0, b) aliased the index of the same number. The
// worst case is a saturated blue reading back as xterm 255, a near-white grey.

func writeSGR(t *testing.T, seq string) Glyph {
	t.Helper()
	term := New(WithSize(4, 1))
	if _, err := term.Write([]byte(seq + "X")); err != nil {
		t.Fatalf("write: %v", err)
	}
	return term.Cell(0, 0)
}

func TestDirectColourIsDistinguishableFromAPaletteIndex(t *testing.T) {
	blue := writeSGR(t, "\x1b[38;2;0;0;255m").FG
	idx255 := writeSGR(t, "\x1b[38;5;255m").FG

	if blue == idx255 {
		t.Fatalf("rgb(0,0,255) and xterm 255 are both %d: the collision is back", blue)
	}
	if !blue.IsRGB() {
		t.Errorf("rgb(0,0,255) should report IsRGB")
	}
	if idx255.IsRGB() {
		t.Errorf("xterm index 255 must not report IsRGB")
	}
	if r, g, b := blue.RGB(); r != 0 || g != 0 || b != 255 {
		t.Errorf("channels round-tripped as (%d,%d,%d), want (0,0,255)", r, g, b)
	}
	if idx255 != Color(255) {
		t.Errorf("palette index changed representation: got %d, want 255", idx255)
	}
}

// Every channel combination must round-trip, and none may land in the palette
// range or on a Default* sentinel.
func TestDirectColourRoundTripsAndStaysDisjoint(t *testing.T) {
	for _, c := range [][3]uint8{
		{0, 0, 0}, {0, 0, 1}, {0, 0, 255}, {0, 1, 0}, {1, 0, 0},
		{255, 255, 255}, {10, 10, 10}, {255, 0, 0}, {0, 255, 0},
	} {
		got := RGB(c[0], c[1], c[2])
		r, g, b := got.RGB()
		if r != c[0] || g != c[1] || b != c[2] {
			t.Errorf("RGB%v round-tripped as (%d,%d,%d)", c, r, g, b)
		}
		if !got.IsRGB() {
			t.Errorf("RGB%v does not report IsRGB", c)
		}
		if got < 256 {
			t.Errorf("RGB%v = %d collides with the palette range", c, got)
		}
		if got == DefaultFG || got == DefaultBG || got == DefaultCursor {
			t.Errorf("RGB%v collides with a Default sentinel", c)
		}
		if got.ANSI() {
			t.Errorf("RGB%v reports ANSI", c)
		}
	}
}

// Bold brightening keys off FG < 8; a direct colour must never be caught by it.
func TestBoldDoesNotBrightenADirectColour(t *testing.T) {
	g := writeSGR(t, "\x1b[1;38;2;0;0;3m")
	if r, gg, b := g.FG.RGB(); r != 0 || gg != 0 || b != 3 {
		t.Fatalf("bold altered a direct colour: got (%d,%d,%d), want (0,0,3)", r, gg, b)
	}
}

func TestRowMatchesCell(t *testing.T) {
	term := New(WithSize(6, 2))
	if _, err := term.Write([]byte("\x1b[38;2;1;2;3mab\x1b[38;5;9mcd")); err != nil {
		t.Fatalf("write: %v", err)
	}
	row := term.Row(0, nil)
	if len(row) != 6 {
		t.Fatalf("row length %d, want 6", len(row))
	}
	for x := 0; x < 6; x++ {
		if row[x] != term.Cell(x, 0) {
			t.Errorf("row[%d] = %+v, Cell = %+v", x, row[x], term.Cell(x, 0))
		}
	}
}

package vt10x

import "testing"

// TestResizeWiderExtendsTabStops pins xterm's behaviour: growing the screen
// continues the 8-column tab stops across the new columns. resize() used to
// set them on the pre-resize slice, so a tab past the old width ran straight
// to the right edge.
func TestResizeWiderExtendsTabStops(t *testing.T) {
	term := New(WithSize(20, 2))
	term.Resize(48, 2)

	cases := []struct {
		in   string
		want int
	}{
		{"\x1b[1;1H\t", 8},
		{"\x1b[1;17H\t", 24}, // first stop past the old width
		{"\x1b[1;25H\t", 32},
		{"\x1b[1;41H\t", 47}, // no stop left: clamps to the last column
	}
	for _, c := range cases {
		term.Write([]byte(c.in))
		if got := term.Cursor().X; got != c.want {
			t.Errorf("%q after resize 20->48: cursor x=%d, want %d", c.in, got, c.want)
		}
	}
}

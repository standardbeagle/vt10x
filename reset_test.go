package vt10x

import "testing"

// RIS (ESC c) clears the whole screen. reset passed rows and cols to clear in
// (x, y) order, so on a screen wider than it is tall only the first rows-many
// columns were cleared.
func TestResetClearsEveryCell(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {24, 80}, {160, 48}, {1, 1}} {
		cols, rows := size[0], size[1]
		term := New(WithSize(cols, rows)).(*terminal)
		for y := 0; y < rows; y++ {
			term.Write([]byte("\x1b[" + itoa(y+1) + ";1H"))
			for x := 0; x < cols; x++ {
				term.Write([]byte("#"))
			}
		}
		term.Write([]byte("\x1bc"))
		for y := 0; y < rows; y++ {
			for x := 0; x < cols; x++ {
				if c := term.Cell(x, y).Char; c != ' ' {
					t.Fatalf("%dx%d: cell %d,%d = %q after RIS", cols, rows, x, y, c)
				}
			}
		}
	}
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

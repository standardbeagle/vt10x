package vt10x

import "testing"

// feed writes s to a fresh-or-existing terminal and fails the test on error.
func feed(t testing.TB, term Terminal, s string) {
	t.Helper()
	if _, err := term.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
}

// cursorAt returns the cursor as (x, y).
func cursorAt(term Terminal) [2]int {
	c := term.Cursor()
	return [2]int{c.X, c.Y}
}

// rowText returns the text of row y.
func rowText(term Terminal, y int) string {
	cols, _ := term.Size()
	var out []rune
	for x := 0; x < cols; x++ {
		g := term.Cell(x, y)
		out = append(out, g.Char)
	}
	return string(out)
}

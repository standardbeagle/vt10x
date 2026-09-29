package oracle

import (
	"strings"
	"testing"
)

// corpus is the hand-written differential set. Each entry is one stream fed to
// both emulators on a 20x6 screen unless the name says otherwise.
var corpus = []struct{ name, in string }{
	// The three defects this module was written to pin.
	{"kitty push", "ab\x1b[>1ucd"},
	{"kitty pop", "ab\x1b[<ucd"},
	{"kitty query", "ab\x1b[?ucd"},
	{"kitty set", "ab\x1b[=1;1ucd"},
	{"modifyOtherKeys", "\x1b[31mab\x1b[>4;2mcd"},
	{"DA2", "ab\x1b[>ccd"},
	{"DA3", "ab\x1b[=ccd"},
	{"DA1", "ab\x1b[ccd"},
	{"sync update", "ab\x1b[?2026hcd\x1b[?2026l"},
	{"XTVERSION", "ab\x1b[>0qcd"},
	{"save restore", "ab\x1b[sxx\x1b[uc"},
	{"esc save restore", "ab\x1b7xx\x1b8c"},
	{"a world of a wide", "a世b"},
	{"zwj", "👩\u200d💻x"},
	{"scroll region cud", "\x1b[2;5r\x1b[3;1H\x1b[50B"},
	{"scroll region cuu", "\x1b[2;5r\x1b[3;1H\x1b[50A"},
	{"scroll region cnl", "\x1b[2;5r\x1b[3;4H\x1b[50E"},
	{"scroll region cpl", "\x1b[2;5r\x1b[3;4H\x1b[50F"},
	{"below region cud", "\x1b[2;5r\x1b[6;3H\x1b[50B"},
	{"above region cud", "\x1b[2;5r\x1b[1;3H\x1b[50B"},
	{"above region cuu", "\x1b[2;5r\x1b[1;3H\x1b[50A"},
	{"below region cuu", "\x1b[2;5r\x1b[6;3H\x1b[50A"},
	{"below region cpl", "\x1b[2;5r\x1b[6;3H\x1b[50F"},
	{"above region cnl", "\x1b[2;5r\x1b[1;3H\x1b[50E"},
	{"tabs without pending wrap", "a\tb\tc\x1b[2Zy"},

	// Ink-style redraw: erase upward, full-width rule, CRLF.
	{"ink redraw", strings.Repeat("\x1b[2K\x1b[1A", 3) + "\x1b[2K\x1b[G" +
		"\x1b[38;2;215;119;87m✻\x1b[39m Thinking…\r\n" +
		"\x1b[38;2;136;136;136m" + strings.Repeat("─", 20) + "\x1b[39m\r\n" +
		"❯ \x1b[7m \x1b[27m\r\n" +
		"\x1b[38;2;136;136;136m" + strings.Repeat("─", 20) + "\x1b[39m\r\n" +
		"  ⏵⏵ bypass · 日本語 👩\u200d💻\r\n"},
	{"ink redraw twice", strings.Repeat("\x1b[2K\x1b[1A", 2) + "\x1b[2K\x1b[Gone\r\ntwo\r\n" + strings.Repeat("\x1b[2K\x1b[1A", 2) + "\x1b[2K\x1b[Gone\r\ntwo\r\n"},

	// Text.
	{"ascii", "hello world"},
	{"cjk", "日本語のテキスト"},
	{"cjk overflow wraps", "日本語のテキストがとても長い行です"},
	{"fullwidth latin", "Ａｂｃ"},
	{"emoji", "😀😃😄"},
	{"emoji keycap", "1\ufe0f\u20e3x"},
	{"skin tone", "👍🏽👍🏿x"},
	{"flag", "🇯🇵🇩🇪🇺🇸x"},
	{"vs16", "❤\ufe0f☺\ufe0fx"},
	{"combining", "e\u0301a\u0308o\u0302x"},
	{"stacked marks", "a\u0301\u0323\u0308x"},
	{"hangul jamo", "\u1112\u1161\u11abx"},
	{"hangul syllables", "한글"},
	{"zwj family", "👨\u200d👩\u200d👧\u200d👦x"},
	{"box drawing", "┌──┐\r\n│ab│\r\n└──┘"},
	{"latin1 and greek", "éàüαβγ"},
	{"emoji then cjk then ascii", "😀世a"},
	{"combining at line start", "\u0301x"},
	{"zwj alone", "a\u200db"},

	// Overwrite / erase / insert / delete around wide characters.
	{"overwrite head", "世世\ra"},
	{"overwrite continuation", "世世\x1b[2Gb"},
	{"wide over halves", "世世\x1b[2G日"},
	{"wide over head", "世a\r日"},
	{"el0 from continuation", "世世\x1b[2G\x1b[K"},
	{"el1 through head", "世世\x1b[1G\x1b[1K"},
	{"el1 through continuation", "世世\x1b[2G\x1b[1K"},
	{"el2", "世世\x1b[2G\x1b[2K"},
	{"ed0", "a世\x1b[3G\x1b[J"},
	{"ed1", "a世\x1b[3G\x1b[1J"},
	{"ed2", "a世\x1b[2J"},
	{"ech on head", "世a\x1b[1G\x1b[X"},
	{"ech on continuation", "世a\x1b[2G\x1b[X"},
	{"ech spans", "a世世\x1b[1G\x1b[4X"},
	{"ich at continuation", "世a\x1b[2G\x1b[@"},
	{"ich at head", "世a\x1b[1G\x1b[@"},
	{"dch at continuation", "世a\x1b[2G\x1b[P"},
	{"dch at head", "世a\x1b[1G\x1b[P"},
	{"dch ends on continuation", "a世b\x1b[1G\x1b[2P"},
	{"dch big", "a世世b\x1b[2G\x1b[9P"},
	{"ich big", "a世世b\x1b[2G\x1b[3@"},
	{"ich zero", "abc\x1b[1G\x1b[0@"},
	{"dch zero", "abc\x1b[1G\x1b[0P"},
	{"ech zero", "abc\x1b[1G\x1b[0X"},

	// Cursor, margins, wrap, tabs.
	{"autowrap boundary", strings.Repeat("a", 20) + "b"},
	{"autowrap exact", strings.Repeat("a", 20)},
	{"autowrap off", "\x1b[?7l" + strings.Repeat("a", 25) + "z"},
	{"cjk exact fill", strings.Repeat("世", 10) + "x"},
	{"decsc decrc attrs", "\x1b[31mab\x1b7\x1b[0mcd\x1b8e"},
	{"scroll region text", "\x1b[2;4r" + strings.Repeat("line\r\n", 8)},
	{"scroll region su sd", "\x1b[2;4r1\r\n2\r\n3\r\n4\r\n5\r\n\x1b[S\x1b[T\x1b[2S"},
	{"il dl", "1\r\n2\r\n3\r\n4\x1b[2;1H\x1b[L\x1b[2M"},
	{"cursor moves", "\x1b[3;5Habc\x1b[2A\x1b[3D\x1b[2C\x1b[4B\x1b[9G\x1b[2d"},
	{"cup default", "abc\x1b[H\x1b[;5H\x1b[3;H"},
	{"tabs", "a\tb\tc\x1b[Ix\x1b[2Zy"},
	{"backspace", "ab\b\bcd\b"},
	{"bs over wide", "世\bx"},
	{"lf cr", "ab\ncd\rx"},
	{"sgr truecolor", "\x1b[38;2;1;2;3;48;2;4;5;6mab\x1b[0mc"},
	{"sgr 256", "\x1b[38;5;196mab\x1b[48;5;21mc\x1b[m"},
	{"reset", "abc\x1bcxyz"},
	{"alt screen", "abc\x1b[?1049hxyz\x1b[?1049lq"},
	{"decawm toggle", "\x1b[?7l" + strings.Repeat("x", 22) + "\x1b[?7h" + strings.Repeat("y", 3)},
	{"osc title", "ab\x1b]0;title\x07cd"},
	{"osc st", "ab\x1b]0;title\x1b\\cd"},
	{"dsr", "ab\x1b[5n\x1b[6ncd"},
	{"cursor hide", "ab\x1b[?25lcd\x1b[?25h"},
	{"mouse modes", "ab\x1b[?1000h\x1b[?1006hcd\x1b[?1000l"},
	{"bracketed paste mode", "ab\x1b[?2004hcd\x1b[?2004l"},
	{"decscusr", "ab\x1b[2 qcd"},
	{"decstbm reset", "\x1b[2;5r\x1b[rab"},
}

func TestCorpusAgainstXVT(t *testing.T) {
	for _, c := range corpus {
		c := c
		t.Run(c.name, func(t *testing.T) {
			compare(t, 20, 6, c.in, exemptions[c.name])
		})
	}
}

// compare feeds in to both emulators and fails on any difference. want is the
// documented reason x/vt is expected to differ from xterm here, or "".
func compare(t *testing.T, cols, rows int, in string, exempt string) {
	t.Helper()
	mine, _ := runVT10x(cols, rows, []string{in})
	oracle := runXVT(cols, rows, in)
	d := mine.diff(oracle)
	if exempt != "" {
		if d == "" {
			t.Errorf("exemption %q no longer needed for %q", exempt, in)
		}
		return
	}
	if d != "" {
		t.Fatalf("%s\ninput %q\nvt10x:\n%s\nx/vt:\n%s", d, in, mine, oracle)
	}
}

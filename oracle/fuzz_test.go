package oracle

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The fuzzer builds streams out of tokens, feeds vt10x one token per Write and
// x/vt the concatenation in a single Write, and compares the final screens. A
// divergence is minimised by dropping tokens and reported with both screens.
// Seeds are fixed, so a failure reproduces exactly.

const (
	fuzzCols = 20
	fuzzRows = 6
)

var (
	asciiWords = []string{"a", "b", "Z", "hello", "x", "12", "~", " ", "  ", "foo bar"}
	cjkTokens  = []string{"世", "界", "日本", "語", "한", "글", "中文", "Ａ", "ア"}
	// Emoji are given both whole and split at every point a Write could split
	// them, since vt10x completes clusters across Write calls.
	emojiTokens = []string{
		"😀", "🎉", "👍", "🚀", "❤\ufe0f", "☺\ufe0f", "👍🏽", "🇯🇵", "👩\u200d💻", "👨\u200d👩\u200d👧",
		"🇯", "🇵", "👩", "💻", "🏽",
	}
	// VS16 is only drawn attached to a symbol that has an emoji form (in
	// emojiTokens): on an arbitrary base uniseg (width 1) and x/vt's
	// displaywidth (width 2) disagree, and terminals do too.
	markTokens = []string{"\u0301", "\u0323", "\u0308", "\u200d"}
	boxTokens  = []string{"─", "│", "┌", "█", "…", "·", "✻", "⏵", "❯"}
	ctrlTokens = []string{"\r", "\n", "\r\n", "\b", "\t"}
)

type suite struct {
	name  string
	seed  int64
	cases int
	// token classes and weights
	weights map[string]int
	// csi filters which CSI generators are allowed
	csi map[string]bool
}

func (s suite) pickClass(r *rand.Rand) string {
	total := 0
	for _, w := range s.weights {
		total += w
	}
	n := r.Intn(total)
	// map iteration order is random; sort by name for reproducibility
	names := make([]string, 0, len(s.weights))
	for k := range s.weights {
		names = append(names, k)
	}
	sortStrings(names)
	for _, k := range names {
		if n < s.weights[k] {
			return k
		}
		n -= s.weights[k]
	}
	panic("unreachable")
}

func sortStrings(a []string) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}

func pick(r *rand.Rand, a []string) string { return a[r.Intn(len(a))] }

func (s suite) csiToken(r *rand.Rand) string {
	var gens []string
	for name, ok := range s.csi {
		if ok {
			gens = append(gens, name)
		}
	}
	sortStrings(gens)
	n := func(max int) int { return 1 + r.Intn(max) }
	switch g := gens[r.Intn(len(gens))]; g {
	case "cuu":
		return fmt.Sprintf("\x1b[%dA", n(8))
	case "cud":
		return fmt.Sprintf("\x1b[%dB", n(8))
	case "cuf":
		return fmt.Sprintf("\x1b[%dC", n(8))
	case "cub":
		return fmt.Sprintf("\x1b[%dD", n(8))
	case "cnl":
		return fmt.Sprintf("\x1b[%dE", n(8))
	case "cpl":
		return fmt.Sprintf("\x1b[%dF", n(8))
	case "cha":
		return fmt.Sprintf("\x1b[%dG", n(fuzzCols))
	case "vpa":
		return fmt.Sprintf("\x1b[%dd", n(fuzzRows))
	case "cup":
		return fmt.Sprintf("\x1b[%d;%dH", n(fuzzRows), n(fuzzCols))
	case "el":
		return fmt.Sprintf("\x1b[%dK", r.Intn(3))
	case "ed":
		return fmt.Sprintf("\x1b[%dJ", r.Intn(3))
	case "ech":
		return fmt.Sprintf("\x1b[%dX", n(6))
	case "ich":
		return fmt.Sprintf("\x1b[%d@", n(6))
	case "dch":
		return fmt.Sprintf("\x1b[%dP", n(6))
	case "il":
		return fmt.Sprintf("\x1b[%dL\r", n(3)) // CR after: x/vt homes the column itself, vt10x does not
	case "dl":
		return fmt.Sprintf("\x1b[%dM\r", n(3))
	case "su":
		return fmt.Sprintf("\x1b[%dS", n(3))
	case "sd":
		return fmt.Sprintf("\x1b[%dT", n(3))
	case "decstbm":
		// Homing after DECSTBM: x/vt leaves the cursor on the top margin, xterm
		// and tmux (and vt10x) put it at the screen's home when origin mode is off.
		if r.Intn(4) == 0 {
			return "\x1b[r\x1b[1;1H"
		}
		top := n(fuzzRows - 1)
		return fmt.Sprintf("\x1b[%d;%dr\x1b[1;1H", top, top+1+r.Intn(fuzzRows-top))
	case "decsc":
		return pick(r, []string{"\x1b7", "\x1b8"})
	case "sgr":
		return pick(r, []string{"\x1b[0m", "\x1b[m", "\x1b[1m", "\x1b[4m", "\x1b[7m", "\x1b[27m", "\x1b[31m", "\x1b[39m", "\x1b[42m",
			fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r.Intn(256), r.Intn(256), r.Intn(256)), fmt.Sprintf("\x1b[48;5;%dm", r.Intn(256))})
	case "private":
		return pick(r, []string{"\x1b[>1u", "\x1b[<u", "\x1b[?u", "\x1b[=1;1u", "\x1b[>4;2m", "\x1b[>c", "\x1b[=c", "\x1b[c",
			"\x1b[>0q", "\x1b[?2026h", "\x1b[?2026l", "\x1b[?25l", "\x1b[?25h", "\x1b[2 q", "\x1b[?2004h", "\x1b[?1004h", "\x1b[?1000h"})
	case "autowrap":
		return pick(r, []string{"\x1b[?7l", "\x1b[?7h"})
	case "cbt":
		return fmt.Sprintf("\x1b[%dZ", n(3))
	case "cht":
		return fmt.Sprintf("\x1b[%dI", n(3))
	}
	panic("unknown csi generator")
}

// token draws the next token. A zero-width mark is only drawn straight after a
// text token: with nothing to attach to, x/vt keeps it in a cell of its own
// while xterm and vt10x drop it, which is not a difference worth comparing.
func (s suite) token(r *rand.Rand, prev string) string {
	class := s.pickClass(r)
	if class == "mark" && !isText(prev) {
		class = "ascii"
	}
	switch class {
	case "ascii":
		return pick(r, asciiWords)
	case "cjk":
		return pick(r, cjkTokens)
	case "emoji":
		return pick(r, emojiTokens)
	case "mark":
		return pick(r, markTokens)
	case "box":
		return pick(r, boxTokens)
	case "ctrl":
		return pick(r, ctrlTokens)
	case "csi":
		return s.csiToken(r)
	}
	panic("unknown class")
}

func allCSI(except ...string) map[string]bool {
	m := map[string]bool{}
	for _, k := range []string{"cuu", "cud", "cuf", "cub", "cnl", "cpl", "cha", "vpa", "cup", "el", "ed", "ech", "ich", "dch", "il", "dl",
		"su", "sd", "decstbm", "decsc", "sgr", "private", "autowrap", "cbt", "cht"} {
		m[k] = true
	}
	for _, k := range except {
		delete(m, k)
	}
	return m
}

var suites = []suite{
	{
		name: "narrow: ASCII and every CSI, no wide characters", seed: 1, cases: 4000,
		weights: map[string]int{"ascii": 30, "ctrl": 8, "csi": 40, "box": 4},
		csi:     allCSI(),
	},
	{
		name: "wide: CJK, emoji, ZWJ, marks with cursor and erase CSI", seed: 2, cases: 4000,
		weights: map[string]int{"ascii": 10, "cjk": 14, "emoji": 14, "mark": 8, "box": 4, "ctrl": 6, "csi": 30},
		csi:     allCSI(),
	},
	{
		name: "wide-heavy: mostly text, few controls", seed: 3, cases: 4000,
		weights: map[string]int{"ascii": 12, "cjk": 20, "emoji": 20, "mark": 12, "box": 6, "ctrl": 6, "csi": 8},
		csi:     allCSI(),
	},
}

// stream joins tokens; x/vt gets it whole.
func stream(tokens []string) string { return strings.Join(tokens, "") }

// diverges reports whether the two emulators disagree on tokens, and the
// description. A stream that hits a known x/vt hazard never diverges.
func diverges(tokens []string) (string, bool) {
	mine, h := runVT10x(fuzzCols, fuzzRows, tokens)
	if h.any() {
		return "", false
	}
	theirs := runXVT(fuzzCols, fuzzRows, stream(tokens))
	d := mine.diff(theirs)
	return d, d != ""
}

// minimise drops tokens while the streams still diverge.
func minimise(tokens []string) []string {
	for changed := true; changed; {
		changed = false
		for chunk := len(tokens) / 2; chunk >= 1; chunk /= 2 {
			for i := 0; i+chunk <= len(tokens); {
				cand := append(append([]string{}, tokens[:i]...), tokens[i+chunk:]...)
				if _, bad := diverges(cand); bad {
					tokens = cand
					changed = true
				} else {
					i += chunk
				}
			}
		}
	}
	return tokens
}

// seedOffset shifts every suite's seed (VT10X_ORACLE_SEED_OFFSET=n) to explore
// beyond the fixed streams; the default 0 is what CI runs.
func seedOffset() int64 {
	v, _ := strconv.ParseInt(os.Getenv("VT10X_ORACLE_SEED_OFFSET"), 10, 64)
	return v * 1000
}

func maxReports() int {
	if v, err := strconv.Atoi(os.Getenv("VT10X_ORACLE_REPORTS")); err == nil && v > 0 {
		return v
	}
	return 3
}

func TestFuzzAgainstXVT(t *testing.T) {
	for _, su := range suites {
		su := su
		t.Run(su.name, func(t *testing.T) {
			r := rand.New(rand.NewSource(su.seed + seedOffset()))
			skipped, reports := 0, 0
			for i := 0; i < su.cases && reports < maxReports(); i++ {
				n := 3 + r.Intn(30)
				tokens := make([]string, n)
				prev := ""
				for j := range tokens {
					tokens[j] = su.token(r, prev)
					prev = tokens[j]
				}
				if _, h := runVT10x(fuzzCols, fuzzRows, tokens); h.any() {
					skipped++
					continue
				}
				if _, bad := diverges(tokens); !bad {
					continue
				}
				min := minimise(tokens)
				d, _ := diverges(min)
				mine, _ := runVT10x(fuzzCols, fuzzRows, min)
				theirs := runXVT(fuzzCols, fuzzRows, stream(min))
				t.Errorf("case %d (seed %d) diverges: %s\nminimal tokens: %q\nvt10x:\n%s\nx/vt:\n%s", i, su.seed, d, min, mine, theirs)
				reports++
			}
			t.Logf("%d cases, %d skipped for known x/vt hazards", su.cases, skipped)
		})
	}
}

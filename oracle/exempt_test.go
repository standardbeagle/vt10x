package oracle

// exemptions lists corpus cases where charmbracelet/x/vt disagrees with xterm,
// and why vt10x follows xterm. Each was checked against xterm's documented
// behaviour and against tmux (an independent emulator that follows xterm), and
// each entry is asserted to still diverge: when x/vt is fixed the corpus test
// says so and the entry should be deleted.
//
// The fuzzer cannot list cases, so it detects the same situations while it
// runs (hazards in harness_test.go) and skips those streams.
var exemptions = map[string]string{
	"save restore": "x/vt does not implement SCOSC/SCORC (CSI s / CSI u); xterm does while DECLRMM is off",

	"ink redraw":         exactFillReason,
	"cjk overflow wraps": exactFillReason,
	"cjk exact fill":     exactFillReason,

	"emoji keycap": "1 + VS16 + U+20E3: uniseg (which the brief mandates) gives width 1, displaywidth (x/vt) gives 2; terminals disagree",

	"ich at head": shiftWideReason,
	"ich big":     shiftWideReason,

	"ich zero": "x/vt reads a zero count literally; xterm and tmux read 0 as 1",
	"dch zero": "x/vt reads a zero count literally; xterm and tmux read 0 as 1",

	"above region cud": aboveRegionReason,
	"above region cnl": aboveRegionReason,
	"below region cuu": belowRegionReason,
	"below region cpl": belowRegionReason,

	"tabs": "x/vt keeps the pending wrap across CBT (CSI Z); xterm and tmux clear it when the cursor moves",
}

const (
	aboveRegionReason = "a cursor above the scroll region moving down stops at the bottom margin in xterm (cursor.c CursorDown: max = cur_row > bot_marg ? max_row : bot_marg) and tmux; x/vt runs on to the screen bottom"
	belowRegionReason = "a cursor below the scroll region moving up stops at the top margin in xterm (cursor.c CursorUp: min = cur_row < top_marg ? 0 : top_marg) and tmux; x/vt runs on to the screen top"
	exactFillReason   = "x/vt does not set pending wrap when a wide character ends exactly at the right margin, so the next character overwrites its continuation and destroys it; xterm and tmux set the pending wrap"
	shiftWideReason   = "x/vt shifts the two halves of a wide character independently on ICH and destroys it; xterm and tmux shift the pair intact"
)

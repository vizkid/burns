package spawn

import (
	"context"
	"errors"
)

// ErrNoCap means the nav was not started, because the cap has no pane to
// split beside.
var ErrNoCap = errors.New("not started: the cap has no pane to split beside")

// Pair starts a cap in a new tab, then a nav beside the cap's block
// (docs/SPEC.md, "Pair"). The nav goes by block ID, not label, so a label
// Rex has not shown yet cannot misplace it. The nav starts whenever the cap's
// pane exists, even if the cap's agent waits, so the layout is whole.
func Pair(ctx context.Context, local, target Rex, cap, nav Request) ([2]Result, [2]error) {
	var res [2]Result
	var errs [2]error
	res[0], errs[0] = Spawn(ctx, local, target, cap)
	if res[0].Block == "" {
		res[1], errs[1] = Result{Label: nav.Label}, ErrNoCap
		return res, errs
	}
	nav.Beside, nav.Anchor, nav.Tab = false, "", ""
	nav.anchorSession, nav.anchorBlock = res[0].SessionID, res[0].Block
	res[1], errs[1] = Spawn(ctx, local, target, nav)
	return res, errs
}

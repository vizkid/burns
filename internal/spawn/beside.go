package spawn

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// splits reports whether the new pane splits an existing tab instead of
// opening one.
func (r Request) splits() bool { return r.Beside || r.Anchor != "" }

// splitBeside splits beside the caller's pane, or beside the pane labelled
// Anchor in the caller's session on target (docs/SPEC.md, "Anchor").
func splitBeside(ctx context.Context, local, target Rex, req Request, res Result) (Result, error) {
	session, block := req.CallerSession, req.CallerBlock
	if req.Anchor == "" {
		if err := sameServer(ctx, local, target); err != nil {
			return res, err
		}
	} else {
		var err error
		if session, block, err = anchorPane(ctx, target, res.SessionLabel, req.Anchor); err != nil {
			return res, err
		}
	}
	if err := labelFree(ctx, target, session, req.Label); err != nil {
		return res, err
	}
	res.SessionID = session
	var err error
	res.Block, err = created(target.Run(ctx, append([]string{"split", "-s", session, "-b", block,
		"--label", req.Label, "--focus=false", "--json"}, command(req)...)...))
	return res, err
}

// anchorPane finds the one pane labelled anchor in the one session labelled
// sessionLabel on target. Anything else would be a guess.
func anchorPane(ctx context.Context, target Rex, sessionLabel, anchor string) (string, string, error) {
	ids, err := sessionsLabelled(ctx, target, sessionLabel)
	if err != nil {
		return "", "", err
	}
	switch len(ids) {
	case 0:
		return "", "", fmt.Errorf("that host has no session labelled %q, so no pane to anchor beside", sessionLabel)
	case 1:
	default:
		return "", "", fmt.Errorf("%d sessions are labelled %q on that host (%s); spawn will not guess", len(ids), sessionLabel, strings.Join(ids, ", "))
	}
	blocks, err := target.Blocks(ctx, ids[0])
	if err != nil {
		return "", "", err
	}
	var found []string
	for _, b := range blocks {
		if b.Label == anchor {
			found = append(found, b.ID)
		}
	}
	switch len(found) {
	case 0:
		return "", "", fmt.Errorf("session %q has no pane labelled %q to anchor beside", sessionLabel, anchor)
	case 1:
		return ids[0], found[0], nil
	}
	return "", "", fmt.Errorf("%d panes in session %q are labelled %q (%s); spawn will not guess", len(found), sessionLabel, anchor, strings.Join(found, ", "))
}

func sameServer(ctx context.Context, local, target Rex) error {
	a, err := local.Whoami(ctx)
	if err != nil {
		return err
	}
	b, err := target.Whoami(ctx)
	if err != nil {
		return err
	}
	if a.HostID != b.HostID {
		return errors.New("--beside splits the caller's own tab, which exists only on the caller's host; use --anchor to split beside a pane there")
	}
	return nil
}

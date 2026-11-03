# Burns

`docs/SPEC.md` is the source of truth. Update it when behavior changes.

- Go code lives in `cmd/` and `internal/`. Run `make check` and `make build` after code changes.
- Keep every Go file under 300 lines. `make check` fails otherwise. Split along real seams.
- Use synthetic fixtures. Keep credentials, personal account names, machine names, and private paths out of the repository.
- Unknown or stale usage is never zero or unlimited. Keep quota separate from spend.
- Burns reads documented CLI output and the CLIs' own local files. It does not scrape screens, cookies, or other processes' environments. Three exceptions: a launch reads its own pane through `rex capture` to answer Claude's channels dialog (Viz, 2026-10-31), `smithers spawn` reads the pane it opened for 5 seconds to see Claude's folder-trust dialog (Viz, 2026-11-03), and `smithersd` reads Claude's undocumented usage endpoint with Claude Code's stored login, and Codex's undocumented daily credit report with Codex's stored login, to report billed spend (Viz, 2026-11-02).
- `smithersd` launches nothing and changes no agent configuration.
- Label proposals separately from implemented behavior.
- Use Conventional Commit messages.

# Burns

`docs/SPEC.md` is the source of truth. Update it when behavior changes.

- Go code lives in `cmd/` and `internal/`. Run `make check` and `make build` after code changes.
- Keep every Go file under 300 lines. `make check` fails otherwise. Split along real seams.
- Use synthetic fixtures. Keep credentials, personal account names, machine names, and private paths out of the repository.
- Unknown or stale usage is never zero or unlimited. Keep quota separate from spend.
- Burns reads documented CLI output and the CLIs' own local files. It does not scrape screens, cookies, or other processes' environments.
- `smithersd` launches nothing and changes no agent configuration.
- Label proposals separately from implemented behavior.
- Use Conventional Commit messages.

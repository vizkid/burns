# Burns

Burns shows quota and token usage for coding-agent accounts, and launches new agent sessions into Rex.
It is in design. Read the draft specification in `docs/SPEC.md`.

Burns has three parts:

- `smithersd` runs on every host and collects local Claude and Codex usage.
- Burns for Mac shows usage from every host and opens the New Agent Session picker.
- Burns for iOS shows usage.

`burns.py` is an early Python preview of the provider and account picker. The Go and Swift apps replace it.

Rex owns hosts, sessions, and panes. Ping owns message delivery. Cop owns credential access.
Burns must compose with these tools without requiring Penfold.
Personal account names, paths, and hostnames belong in local configuration.

Burns is released under the MIT License. See `LICENSE`.

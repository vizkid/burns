# Burns

Burns shows quota and token usage for coding-agent accounts, and launches new agent sessions into Rex.
It is in design. Read the draft specification in `docs/SPEC.md`.

Burns has three parts:

- `smithersd` runs on every host and collects local Claude and Codex usage.
- Burns for Mac shows usage from every host and opens the New Agent Session picker.
- Burns for iOS shows usage.

## smithersd

`smithersd` is built. It collects Claude and Codex usage on one host and serves it on `127.0.0.1:9766`.
It needs Go 1.26.

```sh
make check
mkdir -p ~/.config/burns
cp docs/accounts.example.json ~/.config/burns/accounts.json   # then edit the accounts
make install    # binaries into ~/.local/bin; on macOS, a login agent
curl -s http://127.0.0.1:9766/usage
```

Agents feed it through `smithers`: the statusline forwarder and the SessionStart hook.
`docs/SPEC.md` lists the routes, the sources, and the setup each agent needs (see "Installing").

`burns.py` is an early Python preview of the provider and account picker. The Go and Swift apps replace it.

Rex owns hosts, sessions, and panes. Roar owns message delivery. Cop owns credential access.
Burns must compose with these tools without requiring Penfold.
Personal account names, paths, and hostnames belong in local configuration.

Burns is released under the MIT License. See `LICENSE`.

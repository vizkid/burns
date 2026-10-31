# Burns

Burns collects quota and token usage for Claude and Codex accounts.
See the [design](docs/SPEC.md).

| Part | Job | Status |
|---|---|---|
| `smithersd` | Collects usage on each host | Built |
| Burns for Mac | Shows usage from every host and starts agent sessions | Planned |
| Burns for iOS | Shows usage | Planned |

## Install smithersd

You need Go 1.26.5 or later.

```sh
mkdir -p ~/.config/burns
cp docs/accounts.example.json ~/.config/burns/accounts.json
```

Edit `~/.config/burns/accounts.json` to list your accounts and their agent homes.
To have an agent write it for you, paste the [setup prompt](#setup-prompt) into Claude Code or Codex on that host.
Then install:

```sh
make install
~/.local/bin/smithers usage
```

`make install` copies `smithersd` and `smithers` to `~/.local/bin`.
On macOS, it starts `smithersd` now and at every login.
On Linux, it runs `smithersd` as a systemd user service.
`smithers usage` prints each account's quota and its consumption over the last 24 hours. Add `--since 1h` for another window.

To send usage from your agents, follow the [agent setup](docs/SPEC.md#installing).

`smithersd` also answers the read routes on its Tailscale address, port 9766, to devices of the Tailscale user who owns the host.

### Setup prompt

Run this in the Burns checkout on each host. The agent asks before it writes anything.

```text
Write ~/.config/burns/accounts.json for Burns on this host. Follow docs/accounts.example.json and
the rules in internal/config/config.go.

1. Find the agent homes. Look for ~/.claude*, ~/.codex*, and any CLAUDE_CONFIG_DIR or CODEX_HOME
   set in my shell startup files, aliases, and scripts. List each home you find.
2. Never open or print credential files such as auth.json or .credentials.json.
3. For each Codex home, read config.toml. If its model provider is Azure, the kind is "azure".
   Every other home is "subscription".
4. Ask me which homes are signed in to the same account. Those share one entry: the main home in
   "home" and the others in "extra_homes". The CLI's default home (~/.claude, ~/.codex) is often
   one of these.
5. Propose a name for each account: lowercase letters, digits, and hyphens, starting with a
   letter, at most 32 characters. Ask me to confirm or change the names.
6. Show me the whole file. Write it only after I agree, with mode 0600. Write homes relative to
   my home directory.
7. Run make install, then ~/.local/bin/smithers usage. If smithersd rejects the file, fix the
   entry it names and run both again.
```

## Apps and widgets

Burns for Mac (menu bar) and Burns for iOS show a ring per account and the selected account's burndown, in the app and in widgets.
Put your Apple team and identifier domain in `~/.config/burns/signing.env` as `BURNS_TEAM` and `BURNS_ID_DOMAIN`, then:

```sh
scripts/build-apps.sh mac        # installs /Applications/Burns.app
scripts/build-apps.sh ios        # installs on a connected iPhone
```

Add each host's Tailscale name in the app's settings. See [Apps and widgets](docs/SPEC.md#apps-and-widgets-viz-2026-10-29).

Run `make check` before you commit. It also runs the apps' Swift tests on macOS.

`burns.py` is an early prototype of the account picker. The apps will replace it.

## License

MIT. See `LICENSE`.

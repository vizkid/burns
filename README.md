# Burns

Burns tracks quota, tokens and spend for Claude Code and Codex accounts across several hosts, and starts agent sessions with each account's settings.

- Set up a host: [docs/SETUP.md](docs/SETUP.md). It ends with a prompt an agent can follow.
- Design and rules: [docs/SPEC.md](docs/SPEC.md).

| Part | Job | Status |
|---|---|---|
| `smithersd` | Collects usage on each host and serves it on the tailnet | Built |
| `smithers` | The CLI: usage, panes, launch, spawn, statusline and hooks | Built |
| Burns for Mac | Menu bar usage from every host, account settings, the New Agent Session panel | Built |
| Burns for iOS | Usage and widgets | Built |

## Quick start

```sh
mkdir -p ~/.config/burns
cp docs/accounts.example.json ~/.config/burns/accounts.json   # then edit it
make install
~/.local/bin/smithers usage
```

Then connect each agent home and add the optional parts, as [SETUP.md](docs/SETUP.md) describes.

## Commands

| Command | Job |
|---|---|
| `smithers usage [--since 1h]` | Each account's quota, and its requests, tokens and cost over the window |
| `smithers panes [--url http://HOST:9766]` | The live agent panes, with model, context use and compactions |
| `smithers launch --provider P --account NAME [-- ARGS]` | Starts an agent with the account's home, model, credential, title and theme |
| `smithers spawn --provider P --account NAME --label L --tab T` | Starts an agent in the caller's own Rex session. `--host`, `--beside`, `--anchor` and `--pair` place it. |

## Apps

Put your Apple team and identifier domain in `~/.config/burns/signing.env` as `BURNS_TEAM` and `BURNS_ID_DOMAIN`, then:

```sh
scripts/build-apps.sh mac        # installs /Applications/Burns.app
scripts/build-apps.sh ios        # installs on a connected iPhone
```

To open the Mac app's New Agent Session panel from a Rex key, load the action in the Mac's `~/.config/rex/init.lua`, then run `rex config reload`:

```lua
dofile(os.getenv("HOME") .. "/path/to/burns/scripts/rex/new-agent-session.lua")
rex.bind("shift+cmd+a", "burns_new")
rex.bind("alt+shift+cmd+a", "burns_beside")
```

`open burns://new` opens the panel from anywhere.

## Development

Run `make check` and `make build` before you commit. `make check` runs the Go tests, the Swift tests on macOS, and a 300-line limit on Go files. Use synthetic names in fixtures and examples.

`burns.py` is an early prototype of the account picker.

## License

MIT. See `LICENSE`.

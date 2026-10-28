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

Edit `~/.config/burns/accounts.json` to list your accounts and their agent homes. Then install:

```sh
make install
curl -s http://127.0.0.1:9766/usage
```

`make install` copies `smithersd` and `smithers` to `~/.local/bin`.
On macOS, it starts `smithersd` now and at every login.
On Linux, start `~/.local/bin/smithersd` yourself before you run `curl`.

To send usage from your agents, follow the [agent setup](docs/SPEC.md#installing).

Run `make check` before you commit.

`burns.py` is an early prototype of the account picker. The apps will replace it.

## License

MIT. See `LICENSE`.

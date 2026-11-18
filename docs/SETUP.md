# Setting up Burns

Owner: Viz. Last updated 2026-11-18.

This guide sets up Burns on one host. Repeat it on each host that runs Claude Code or Codex. The design and the reasons behind each rule are in [SPEC.md](SPEC.md).

> **One accounts file per host drives everything.** `smithersd` reads it to track usage, and `smithers launch` reads it to start an agent with the account's home, settings, icon and theme.

To have an agent do the setup for you, paste the [agent prompt](#agent-prompt) into Claude Code or Codex in the Burns checkout.

## What you need

| Item | Why |
|---|---|
| Go 1.26.5 or later | Builds `smithersd` and `smithers` |
| macOS, or Linux with a systemd user manager | Runs `smithersd` as a service |
| Claude Code or Codex, signed in | The agents Burns tracks |
| Rex (optional) | Pane binding, launches from the Mac app, `smithers spawn`, pane themes |
| Tailscale (optional) | Lets the apps read this host |
| cop (optional) | Passes a credential, such as an Azure key, to a launch |
| jq (optional) | The shell launchers below read the accounts file with it |

## 1. Write the accounts file

```sh
mkdir -p ~/.config/burns
cp docs/accounts.example.json ~/.config/burns/accounts.json
chmod 600 ~/.config/burns/accounts.json
```

Edit the copy to list your accounts. One entry is one provider and one account name. Give both providers of one person the same `name`, `colour`, `emoji`, `symbol` and `theme`, because the apps group them by name.

| Field | Required | Value |
|---|---|---|
| `provider` | Yes | `claude` or `codex` |
| `name` | Yes | Lowercase letters, digits and hyphens, starting with a letter, at most 32 characters |
| `home` | Yes | The agent home (`CLAUDE_CONFIG_DIR` or `CODEX_HOME`), absolute or relative to your home folder, such as `.claude-work` |
| `extra_homes` | No | Other homes signed in to the same account, such as the default `.claude` or `.codex`. Each home belongs to one account only. |
| `kind` | Yes | `subscription`, or `azure` for a Codex home whose model provider is Azure |
| `colour` | No | `purple`, `teal`, `blue`, `green`, `pink` or `grey`. The apps' colour for the account. |
| `emoji` | No | The icon Rex shows in tab labels and titles: one glyph, such as `◆` |
| `symbol` | No | The SF Symbol the apps draw, such as `diamond.fill`. Set it with the Mac app's icon picker, which also sets `emoji`. |
| `theme` | No | A Rex theme id, such as `petrol-lagoon`, for the account's agent panes. No theme leaves the terminal's colours alone. |
| `launch.model` | No | The model to start with |
| `launch.effort` | No | Claude's effort level. Codex refuses it. |
| `launch.permission_mode` | No | Claude's permission mode, such as `auto`. Codex refuses it. |
| `launch.credential` | No | A secret that cop passes to the agent: cop `account`, `ref` and the `env` variable name. `project` names cop's project for launches outside Rex. The file holds the reference, never the secret. |
| `credit_price` | No | What the account paid per credit, as `currency` (three letters) and `amount` |

Unknown fields are errors. `smithersd` names the entry and the problem when it rejects the file.

## 2. Install

```sh
make install
~/.local/bin/smithers usage
```

`make install` builds `smithersd` and `smithers`, copies them to `~/.local/bin`, and starts `smithersd` as a login agent on macOS or a systemd user service on Linux. Run it again to upgrade. `smithers usage` prints each account's quota and its use over the last 24 hours.

| Item | Where |
|---|---|
| Log on macOS | `~/Library/Logs/burns/smithersd.log` |
| Log on Linux | `journalctl --user -u burns-smithersd` |
| Database | `~/.local/state/burns/smithers.db` |
| Launch log | `~/.local/state/burns/launch.log` |

On Linux, run `loginctl enable-linger $USER` to keep `smithersd` running after you log out.

## 3. Connect the agents

The install changes no agent configuration. Add these in each agent home. All of them are safe on a host without `smithers`.

**Claude statusline.** In the statusline script, send the input to `smithers` in the background, then print your own line:

```sh
input=$(cat)
[ -x "$HOME/.local/bin/smithers" ] && { echo "$input" | "$HOME/.local/bin/smithers" statusline >/dev/null 2>&1 & }
```

**Claude SessionStart hook.** In the home's `settings.json`:

```json
{
  "hooks": {
    "SessionStart": [
      {"hooks": [{"type": "command", "async": true, "timeout": 5,
        "command": "test -x $HOME/.local/bin/smithers && $HOME/.local/bin/smithers hook session-start --provider claude || true"}]}
    ]
  }
}
```

**Codex SessionStart hook.** In the home's `hooks.json`, the same command with `--provider codex`. Codex runs a new or changed hook only after you mark it trusted with `/hooks` in that home.

## 4. Optional parts

### Channel servers

A tool that needs a Claude channel server, such as a messaging tool, declares it in `~/.config/burns/tools.d/<tool>.json`. `smithers launch` passes the declared servers and answers Claude's development-channels dialog for them.

```json
{"claude": {"channel_servers": ["example"]}}
```

### Shell launchers

`smithers launch --no-pause --provider P --account NAME -- ARGS` starts an agent from a shell with everything the accounts file sets. Arguments after `--` go to the agent. This zsh snippet defines `c<name>` for each Claude account and `x<name>` for each Codex account:

```zsh
_burns_launchers() {
    local accounts="${XDG_CONFIG_HOME:-$HOME/.config}/burns/accounts.json"
    [[ -r "$accounts" ]] && (( $+commands[jq] )) || return 0
    local provider name prefix
    jq -r '.accounts[] | "\(.provider) \(.name)"' "$accounts" | while read -r provider name; do
        case "$provider" in claude) prefix=c ;; codex) prefix=x ;; *) continue ;; esac
        [[ "$name" =~ '^[a-z][a-z0-9-]*$' ]] || continue
        eval "${prefix}${name}() { smithers launch --no-pause --provider $provider --account $name -- \"\$@\"; }"
    done
}
_burns_launchers
```

### Themes on a host without Rex

`smithers launch` reads theme colours from the Rex app. On a host without the Rex app, such as a Linux host, copy the themes from a Mac that has it:

```sh
jq -c 'map({id, name, appearance, palette,
  semantic: (.semantic | {terminalBackground, terminalForeground, terminalCursor})})' \
  "/Applications/Rex.app/Contents/Resources/SuperUI_SuperDesignTokens.bundle/Contents/Resources/themes.json" \
  > themes.json
```

Put `themes.json` in `~/.config/burns/` on the other host. Copy it again after a Rex update that changes the themes.

### Apps

The Mac menu bar app and the iPhone app show every host's usage. Put your Apple team and identifier domain in `~/.config/burns/signing.env` as `BURNS_TEAM` and `BURNS_ID_DOMAIN`, then run `scripts/build-apps.sh mac` or `scripts/build-apps.sh ios`. Add each host's Tailscale name in the app's settings. The app's Accounts settings edit the accounts file on every host, including the icon and theme pickers.

## 5. Check it works

| Check | Expected |
|---|---|
| `smithers usage` | One row per account. Quota reads `unknown` until an agent in that home runs. |
| Start Claude in a home, then `smithers usage` | That account shows quota and requests |
| `smithers panes`, inside Rex | The new pane, with its account |
| `cNAME --version` | The title shows the icon, folder and account, and Claude prints its version |

## Agent prompt

Paste this into Claude Code or Codex, started in the Burns checkout on the host you are setting up.

```text
Set up Burns on this host. Follow docs/SETUP.md and use docs/accounts.example.json as the template.
Ask me before you write or change any file outside this checkout, and show me each change first.

1. Check the requirements in "What you need". Report what is missing. Stop if Go is missing.
2. Find the agent homes: ~/.claude*, ~/.codex*, and any CLAUDE_CONFIG_DIR or CODEX_HOME set in my
   shell startup files, aliases and scripts. List each home you find.
3. Never open, print or copy credential files, such as auth.json or .credentials.json, and never
   read a secret value. A credential reference in the accounts file is fine; its value is not.
4. For each Codex home, read config.toml. If its model provider is Azure, the kind is "azure".
   Every other home is "subscription".
5. Ask me which homes are signed in to the same account. Those share one entry: the main home in
   "home" and the others in "extra_homes".
6. Propose a name for each account and ask me to confirm. Ask whether I want a colour, an icon
   glyph, a Rex theme, a default model, or a permission mode for each name. Leave out what I skip.
   Give both providers of one name the same colour, icon and theme.
7. If an Azure or other account needs a key, ask me for the cop account, reference and environment
   variable name. Do not search a vault for them.
8. If ~/.config/burns/accounts.json exists, show me the difference instead of replacing it. Write
   the file only after I agree, with mode 0600 and homes relative to my home folder.
9. Run make install, then ~/.local/bin/smithers usage. If smithersd rejects the file, fix the entry
   it names and run both again.
10. Show me the statusline and SessionStart hook changes from "Connect the agents" for each home,
    merged with what is already there. Apply them after I agree. For Codex, tell me to run /hooks
    in each home and mark the hook trusted.
11. Ask whether I want the shell launchers, and where my shell startup file keeps functions.
    Before you add them, check that no existing function or alias has the same name.
12. If this host has no Rex app and an account has a theme, tell me to copy themes.json from a Mac,
    as in "Themes on a host without Rex".
13. Report what you set up, what you skipped, and the output of smithers usage.
```

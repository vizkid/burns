# Burns

Owner: Viz · Last updated: 2026-10-28 · Status: draft, not ratified

## Problem and solution

Starting an agent session means picking a host, a session, a provider, an account, a model, and an effort.
The right account depends on how much quota each one has left, and that data lives on several hosts.
Today Penfold holds the usage data, and nothing helps with the launch choice.

Burns shows usage for every coding-agent account and launches a new agent session into Rex.
One keypress launches the account with the most quota left, using its saved defaults.
When every quota is exhausted, it launches Azure.

> **Core insight: each host keeps its own latest reading, and the apps read from the hosts.**
> No central daemon must stay awake. A closed app or a sleeping Mac loses nothing.

## Components

| Component | Runs on | Job |
|---|---|---|
| `smithersd` | Every host, Stiletto included | Collects local usage. Keeps the latest reading per account and a short history. Answers read requests over Tailscale. Read-only: it launches nothing. |
| Burns for Mac | Mac menu bar | Reads every `smithersd`, merges readings, shows usage, owns settings, shows the trigger modal, and launches through the `rex` CLI. |
| Burns for iOS | iPhone | Reads `smithersd` directly and shows usage. No launch in this phase. |
| New Agent Session | Rex action in Stiletto's `init.lua` | Opens the trigger modal with the current host and session. |

There is no `burnsd`. Viz decided this on 2026-10-27: the Mac app is the hub when it runs.

Burns is written in Go (`smithersd`) and Swift (both apps), matching Ping, Cop, and Penfold.
The Python preview in `burns.py` is superseded.

## Boundaries

| Owner | Owns |
|---|---|
| Rex | Hosts, sessions, tabs, panes, and terminal transport |
| Ping | Message delivery |
| Cop | Credentials |
| Dotfiles | Agent launch configuration, through the launcher functions |
| Burns | Usage visibility, launch choice, and launch request |

Burns takes over usage from Penfold (Viz, 2026-10-27). Penfold's usage code retires after Burns ships.
The switch is a single cutover, not a period of running both (Viz, 2026-10-27).
At cutover, the Claude telemetry endpoint and the statusline forwarder move from Penfold to `smithersd`.
`smithersd` does not forward data to Penfold.
Burns must not require Penfold.

## Usage model

### Accounts

An account is a provider plus an account name, such as Claude `viz` or Codex `azure`.
Azure exists only for Codex. Local configuration declares the accounts, their agent homes, and their launchers.
Shared source uses example names only.

### Readings

| Reading | Meaning |
|---|---|
| Five-hour window | Percent used and reset time |
| Seven-day window | Percent used and reset time |
| Credits | Whether paid credits are in use, kept apart from quota |
| Token burn | Measured input and output tokens per minute |
| Compactions | Count per agent session, turn, Rex block, and account, with each compaction's own tokens |

Each window carries its own observation time.

Azure has no quota windows. Burns shows it as "Microsoft credits", never as unlimited.
Burns does not track the Azure credit balance yet.

### Rules

These rules come from Penfold's `CLAUDE_QUOTA_INGESTION_SPEC.md` and `TOKEN_BURN_SPEC.md`.
Burns rewrites the code from these rules. Each rule gets a test.

1. Track each window independently. A report that omits one window never erases the other.
2. A window whose reset time has passed is unknown, never zero.
3. A reported zero with a future reset is a real reading.
4. If Claude omits a window, keep the stored reading until its reset passes.
5. If Codex omits the five-hour window, clear it. A Codex account can report only a weekly window.
6. When merging hosts, take the newest live reading per window. Keep source times unchanged.
7. Never infer a percentage or a reset time from token counts.
8. A burn rate needs two samples from one session. Until then it is unavailable.
9. Unknown or stale data never displays as zero or unlimited.

### Sources

Research on 2026-10-27 and 2026-10-28 picked one live source and one durable backfill per provider.
Every source is a documented feature or a local file the CLI writes. Burns scrapes nothing.

| Data | Claude | Codex |
|---|---|---|
| Quota windows | Statusline JSON `rate_limits`, posted to `smithersd` in the background | `token_count.rate_limits` in the rollout: key by `limit_id` and `window_minutes` |
| Credits | No supported source. An exhausted live window means extra usage, as in Penfold. | `token_count.rate_limits.credits`. `balance` is a string. |
| Tokens, live | OTLP log event `claude_code.api_request`, one per API request | `token_usage_record` lines in the rollout, one per response |
| Tokens, backfill | Transcript JSONL `message.usage`, including `<session>/subagents/*.jsonl` | Same rollout file, re-read from the stored offset |
| Cost | `cost_usd_micros` on `api_request`. It is a list-price estimate. | Not in phase 1 |

Neither CLI's OpenTelemetry output carries quota windows.
Codex needs no OpenTelemetry: the rollout carries quota, credits, and tokens.

#### Claude tokens

Use the `api_request` log event, not the token metrics.
Only the log event carries `prompt.id`, `request_id`, `agent_id`, and the 5-minute and 1-hour cache-write split.

| Field | Meaning |
|---|---|
| `session.id` | Agent session. `/clear` replaces it. |
| `prompt.id` | One user turn |
| `request_id` | One API request. Joins to the transcript's `requestId`. |
| `agent_id`, `query_source` | Present on subagent requests, so subagent burn rolls up to its session |
| `input_tokens` | Excludes cache tokens |
| `cache_read_tokens`, `cache_creation_tokens` | Cache tokens. Also `cache_creation_5m_tokens` and `cache_creation_1h_tokens` (seen on the wire, not documented). |
| `cost_usd_micros` | Integer cost. Four live events summed to the CLI's own `total_cost_usd` exactly. |

Claude keeps no buffer. If `smithersd` is down, the events of that time are lost.
The transcript fills the gap: join on `requestId` and add only requests that OTel never delivered.

The transcript writes one line per content block, and each line repeats the request's usage.
On 2026-10-28, 33,304 transcript rows held 20,701 distinct requests.
Deduplicate on `requestId`, falling back to `message.id`, across every file of one Claude home. Keep the row with the largest token total.

The transcript never records the compaction request. A live test on 2026-10-28 found the `query_source=compact` request in OTel but not in the transcript.
This likely explains why one long session's transcript held 2 to 3 percent fewer tokens than Claude's own totals.
So compaction tokens lost by OTel cannot be backfilled. Burns marks them unknown, never zero.

Each launcher must set the telemetry variables. On 2026-10-28, Claude processes started in Rex panes had no telemetry, because only the zsh `claude()` function set the variables.
Burns setup puts them in each Claude home's `settings.json` `env` block, so every start is covered.

#### Codex tokens

Use `token_usage_record`, written since Codex CLI 0.153.4. Its payload has `thread_id`, `session_id`, `turn_id`, `root_turn_id`, `response_id`, and `usage`.

| Field | Meaning |
|---|---|
| `input_tokens` | Includes `cached_input_tokens` |
| `output_tokens` | Includes `reasoning_output_tokens` |
| `session_id` | The root thread. A subagent's records carry its root, so burn rolls up. |

Do not sum `token_count` totals. On 2026-10-28 local rollouts showed three faults:

1. Compaction requests never reach `total_token_usage`. One session was 671,928 tokens short.
2. The total reset after an idle gap three times.
3. `token_count` repeats after compaction, so summing `last_token_usage` double-counts.

Rules:

1. Insert each record keyed by `(thread_id, response_id)`. Skip a key already stored. Re-reading a file then changes nothing.
2. Sum `usage` by `turn_id` for a turn and by `session_id` for a session.
3. Rollouts older than 0.153.4 have no records. Use the growth of the `token_count` total. On a decrease, take `last_token_usage` and start over.
4. Skip a malformed line. Never set a counter to zero.

Find active rollouts every 5 seconds by modification time across the whole `sessions` tree. A resumed session appends to its file in the old date folder.
Check active files' sizes every second. Read only complete lines from the stored offset.
Store path, inode, offset, and a hash of the last bytes read. If any of them stops matching, re-read from 0.
Do not use `fsnotify`: on macOS it is not recursive.

#### Compactions

Burns counts every compaction and the tokens the compaction request itself used.
Both CLIs record compactions in local data, so no extra source is needed.

| | Claude | Codex |
|---|---|---|
| Live event | OTel `compaction` event: `session.id`, `prompt.id`, `trigger` (`auto` or `manual`), `success`, `duration_ms`, `pre_tokens`, `post_tokens` | Rollout `compacted` line |
| Compaction tokens | The `api_request` event with `query_source=compact` | The `token_usage_record` whose `response_id` equals the `compacted` line's `compaction_response_id` |
| Backfill | Transcript `system` line with `subtype=compact_boundary`: `compactMetadata.trigger`, `preTokens`, `postTokens`, `durationMs`. No tokens for the request itself. | Same rollout file |
| Dedupe key | Session and `event.timestamp`; transcript boundary by its `uuid` | `compaction_response_id` |
| Trigger | `auto` or `manual` | Not recorded. Burns shows "unknown". |
| Context before and after | `pre_tokens` and `post_tokens` | Not recorded. The compaction request's `input_tokens` is the context sent to compaction. |

Evidence on 2026-10-28:
- A manual `/compact` on a small Haiku session produced one `compaction` event and one `query_source=compact` request in OTel. The transcript had the boundary but not the request.
- 150 recent Claude transcripts held 78 `compact_boundary` lines, all `auto`.
- Codex rollouts with a `compacted` line: 126 in personal, 109 in azure, 467 lines in nonfx.

A compaction belongs to the turn it ran in. Its tokens count toward that turn, its session, its block, and its account like any other request.
Burns shows the compaction count and the compaction share of tokens for each session.

#### Binding a session to a Rex block

Rex calls a pane a block. Every process in a block inherits `REX_SESSION` and `REX_BLOCK`.
Block IDs are UUIDv7 and scoped to one Rex server, so a binding key is `(host, block)`.
A block can host many agent sessions over time, and a resumed session can move to another block.
So a binding is an interval: host, block, provider, agent session, start, end.

| Path | Provider | How |
|---|---|---|
| Resource attribute | Claude | The Claude home sets `OTEL_RESOURCE_ATTRIBUTES=burns.account=<account>,rex.block=$REX_BLOCK,rex.session=$REX_SESSION`. Every `api_request` names its block. Verified on the wire 2026-10-28. |
| SessionStart hook | Claude and Codex | The hook posts provider, agent session ID, `source`, `REX_SESSION`, `REX_BLOCK`, and its ancestor process IDs to `smithersd`. Claude fires it on startup, resume, clear, compact, and fork. Codex fires it at the first prompt. |
| Check | Both | `smithersd` calls Rex's read-only block `process` method. The block's foreground process must appear in the hook's ancestors. |
| End | Both | One `rex events --all --json` stream per host ends a binding on `block_closed`, or when the block's agent session changes. |

Codex runs a hook only after someone approves it with `/hooks` in that Codex home.
A session with no binding counts as "unattributed" for its account. Burns never drops its tokens.
Burns does not read other processes' environments.

The statusline runs only while a Claude session renders. An idle account's reading ages until its reset.

Sources not used in this phase:

| Source | Why not now |
|---|---|
| `GET api.anthropic.com/api/oauth/usage` | Undocumented. It needs the CLI's OAuth token, which Anthropic's terms restrict. It is the only source of Claude extra usage. |
| Claude token metrics | No `prompt.id`, `request_id`, or cache split. The log event covers them. |
| Claude statusline `cost` | Cumulative per session and reset by `/clear`. The log event is finer. |
| `token_count` totals | Undercount, as above |
| Codex `notify` | No tokens. One program per home, already used for Computer Use. |
| Codex OpenTelemetry | `codex.sse_event` has tokens but no `response_id`. `codex.turn_cost` (0.165) is a cost estimate, unverified on these plans. |
| `codex app-server`, method `account/rateLimits/read` | Gives idle Codex readings. The protocol is marked unstable. Candidate for a later phase. |
| CodexBar | Reference only. It works per user, not per agent home. |
| ccusage | Infers quota blocks from transcripts. It has no real percentages. Its dedupe logic is a reference. |

#### Cost

Cost is out of phase 1 for Codex.
A later phase uses a checked-in price table, with `as_of`, `source_url`, and `basis` on every row.
Azure rows come from the Retail Prices API at `prices.azure.com/api/retail/prices`. Price depends on deployment type and region, so local configuration declares both.
Subscription accounts show cost as "list-price equivalent", never as spend.

### Storage

Each `smithersd` keeps one SQLite file in WAL mode, using `modernc.org/sqlite` (pure Go, no cgo).

| Table | Holds | Kept for |
|---|---|---|
| `latest` | Newest reading per account, `limit_id`, and window, with observation time | Until replaced |
| `request` | One row per Claude request or Codex response: dedupe key, account, agent session, turn, subagent, compaction flag, tokens, cost | 7 days |
| `compaction` | One row per compaction: account, agent session, turn, trigger, context before and after, duration, request key | 7 days |
| `binding` | Host, block, provider, agent session, start, end | 7 days after end |
| `minute` | Token buckets per account | 20 minutes |
| `hour` | Token rollups per account | 7 days |
| `cursor` | Rollout path, inode, offset, tail hash | While the file exists |

Per-session, per-turn, and per-block views are queries over `request` joined to `binding`.

No OpenTelemetry Collector and no Prometheus. They are too large for about 10 accounts.

## The trigger modal

### Context strip

The top of the modal shows the host and session, filled in from where the modal was opened.

| Choice | Default | Options |
|---|---|---|
| Host | Host of the session that opened the modal | Every Rex host plus the local one |
| Session | New tab in the current session | Any existing session on that host, or a new session |
| Directory | Current session's directory | New session only: directories of that host's sessions and known worktrees. No typed paths. |

### Steps

1. Provider: Claude or Codex, shown as large marks with readable labels.
2. Account: viz or nfx. Azure appears only under Codex, with its own mark.
3. Model and effort.

Each provider has a default model and effort. An account can override them.
Azure needs its own, because its models are deployment names, such as `gpt-6-astra`.
Settings edit both levels.

### Default rule

Quota differs for each provider and account pair, so Enter ranks pairs, not providers (Viz, 2026-10-27).

1. List every quota account: each Claude and Codex account except Azure.
2. For each one, quota left is 100 minus the highest percent used across its live windows.
3. An account with an unknown window does not compete.
4. The account with the most quota left wins. A tie goes to the earlier account in the settings order.
5. If no account has quota left, and every quota account is known to be exhausted, choose Azure.
6. If no account has quota left, but some are unknown, choose the default account from settings. The modal says usage is unknown.

Azure has no subscription quota. It runs on Microsoft credits, so it is the fallback when everything else is exhausted.
Exhausted means a live window at 100 percent.
Rule 5 also puts Azure ahead of paid Claude extra usage.

The winning account brings its model and effort defaults.
The modal shows why it picked that account, for example "Codex nfx: 62% left" or "All quotas exhausted: Azure".

### Keyboard

| Key | Action |
|---|---|
| Enter | Accept the selection shown and launch |
| 1, 2, 3 | Choose an option on the current step |
| Tab and Shift+Tab | Move between steps |
| H | Choose the host |
| S | Choose the session |
| Esc | Step back, or close at the first step |

The final key map needs a visual trial.

### Marks

The Mac app uses Penfold's SVG marks for Claude, Codex, and Azure.
The Unicode marks are only for a terminal fallback, if one is built.

## Launch

1. The Rex action finds the server that owns `ctx.session_id` by checking `session.list` on the local server and each server in `rex.servers()`. It then runs `open "burns://new?host=<host>&session=<session>"` on Stiletto.
2. The modal collects the choice.
3. The app resolves the model against the account's catalog. An unknown model is refused before launch.
4. The app runs `rex -S <host>` to create the tab or session. The command is the account's launcher with the model arguments.
5. The app asks Rex to show the new session.
6. The app reports Launched, Confirmed, or Failed, following Penfold's launch confirmation rules.

Request values become launcher arguments, never shell code.
Model catalogs port from Penfold's `internal/models` with their tests.

## Wire protocol

`smithersd` serves HTTP on the tailnet.

| Route | Answer |
|---|---|
| `GET /usage` | Latest readings per account, with observation times |
| `GET /usage/history` | 20 one-minute burn buckets per account |
| `GET /usage/sessions` | Tokens and compactions per agent session, turn, and Rex block |

`smithersd` checks the caller's Tailscale identity. It accepts readings only from local statusline posts and local files.

### Rex action findings (tested 2026-10-27)

| Finding | Evidence |
|---|---|
| Stiletto's action runs on Stiletto, even when started from a Baroni tab. | `hostname` returned `Stiletto.local` for both palette runs. |
| Its commands run in the GUI login session, so `open` works. | `launchctl managername` returned `Aqua`. `open` exited 0. |
| The palette offers Stiletto's actions while a Baroni session is showing. | The second run reported Baroni's session ID. |
| `ctx` gives the session ID but not its server. | The action must look the session up on each server. |
| The requesting client supplies the servers the action can reach. | A palette run listed `Baroni` and `session.list` on Baroni worked. A CLI run, with or without `-C`, listed none. A `rex.host` line in `init.lua` did not change this. |

## Phases

1. `smithersd` collects Claude and Codex usage on one host, and the rules pass their tests.
2. The Mac menu bar app merges every host and shows usage.
3. The trigger modal, settings, and launch through Rex.
4. The New Agent Session Rex action.
5. Burns for iOS shows usage.
6. Penfold's usage code retires.

## Open questions

| Question | Status |
|---|---|
| Does the palette offer Stiletto's actions while the app shows a Baroni session? | Yes. Tested 2026-10-27 with a temporary action. |
| Does `open` from a Rex action open the deeplink on Stiletto? | Yes. Tested 2026-10-27. |
| Should the account also be picked by quota left, not only the provider? | Yes: Enter ranks provider and account pairs. Azure is the fallback. (Viz, 2026-10-27) |
| Should Burns track the Azure credit balance? | Probably yes, but deferred (Viz, 2026-10-27). Not in the current phases. |
| Does the iOS app read hosts from synced settings or its own list? | Open |
| Source license | MIT (Viz, 2026-10-27) |

## Cross-references

- Rex decision record: `rex/decisions.md`, "Burns: picker direction"
- Penfold launch: `docs/specs/LAUNCH_SPEC.md`, sections "Launcher ownership", "Models", "Launch confirmation"
- Penfold quota: `docs/specs/CLAUDE_QUOTA_INGESTION_SPEC.md`, `docs/specs/TOKEN_BURN_SPEC.md`
- Penfold marks: `apps/PenfoldKit/Sources/PenfoldKit/ProviderMark.swift`, `WidgetProvider.swift`

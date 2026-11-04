# Burns

Owner: Viz · Last updated: 2026-11-01 · Status: draft, not ratified

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
| New Agent Session | Rex action `burns_new` in `scripts/rex/new-agent-session.lua`, loaded by the Mac's `init.lua` | Opens the trigger modal on the current host and session. |

There is no `burnsd`. Viz decided this on 2026-10-27: the Mac app is the hub when it runs.

### Showing usage (Viz, 2026-10-29)

| Where | Shape | Source |
|---|---|---|
| Mac menu bar window | A row of Penfold rings at the top, one per account. Selecting a ring shows that account's combined burndown chart below the row. | Viz, 2026-10-29 |
| Widgets (iOS and macOS) | CodexBar's burndown chart, per account: the longest live window, normally the week | Design from CodexBar (see "Cross-references"). Design only; no code is copied. |
| Usage views in both apps | Penfold's rings, one tile per account: outer ring five hours, inner ring seven days, provider mark in the middle | Port of PenfoldKit `SpendTile`, `SpendRings`, and `SpendPace` |

A burndown plots quota left from the window's start to its reset. It charts the account's longest live window, normally the week; the five-hour window is shown only by the ring (Viz, 2026-10-29). If the week is unknown, it charts the five-hour window.

| Element | Meaning |
|---|---|
| Area with a solid edge | Quota left over time, filled in the pace colour, from the window's readings (see "Reading history"). The window starts at its reset minus its length, at 100%. Readings are whole percents, so the line keeps the moment usage first reached each one, and readings closer than 1% of the window merge into the later one. A monotone curve joins them: it never turns back or overshoots a reading. A reading lower than an earlier one in the window is stale and skipped (rule 10). Before the first reading, and with no readings yet, it is a straight line. It ends at the current percent, at the time it was last observed: readings record changes only, so a percent unchanged since the last reading holds flat to that time (Viz, 2026-10-31). |
| Thin faint dashed line | The steady burn: 100% at the window's start to 0% at its reset |
| Header, right side | Percent left, the current reading, the outlook and the time to reset: `34% left · 15% under · 4h ahead of pace · resets 1d 7h`. The current reading is `over` or `under` the steady line, in percent. The outlook is where the projection ends: `4h ahead of pace` when it runs out before the reset, `12% behind pace` when that much is left at the reset, and `at pace` when it ends within half a percent of the window of the reset (Viz, 2026-11-02). A reading can be under the line and ahead of pace. Each part is left out while it has nothing to say, and a spent window shows `Budget spent`. |
| Heavy grey dashed line, faint grey area under it | The current burn projected to the reset, or to 0% left. The burn is the change over the last seventh of the window, a day of a week, measured to now (Viz, 2026-11-02: the window's average did not follow current burn). It starts where the actual line ends, at the last observation, which can be hours old because Codex reports only when a turn runs (Viz, 2026-10-31); the quiet hours since count as burning nothing. Ring colours and verdicts still use the window's average. Verdicts and the projection hold for the first 0.5% of a window, 50 minutes of a week (Viz, 2026-11-02). Same dash as the steady line, grey, with its dashes in step with the steady line's so the two read as one where they meet (Viz, 2026-10-29). |
| Headline | For the charted window: percent left, the pace verdict ("over pace"), and time to reset. "Budget spent" when it reached 0%. |
| Bars behind the chart | A time grid: hours for the five-hour window, days for the week. Elapsed slots are shaded. |

Each ring draws two arcs: the share of quota used in the pace colour, and the share of the window elapsed in a neutral blue-grey. Under pace the elapsed arc sits behind usage and shows past it. Over pace the elapsed arc draws on top of usage, one pace colour down: orange on red, yellow on amber. The usage colour left showing is how far ahead the window is (Viz, 2026-11-01: usage on top hid the pace, and a blue-grey arc did not contrast with red).
The pace colour is yellow on or under pace, amber when 100% is projected with a quarter of the window or less left, and red when it is projected earlier or already reached. The chart's lines use the same colour.
The pace colour sits on usage, as on Penfold's Mac bars, not on the time arc as in Penfold's rings (Viz, 2026-10-29). An amber time arc over a yellow usage arc of nearly the same length hid the yellow and read as a second shade of it.
Rings carry no pace tick (Viz, 2026-10-01, in Penfold). Tiles list each account's Claude then its Codex, in settings order.
Rings and burndowns share `SpendPace`, so they give the same verdict.
An unknown window draws an empty track and says "unknown", never 0%. A gap with no readings draws no line, never a flat one.

StandBy and Home Screen widget (Viz, 2026-10-29): the same layout as the menu bar window. A row of rings, one per account, sits at the top, and the selected account's combined burndown fills the rest.
Each ring is a button. Its App Intent stores that account as the shared selection and reloads the widget, so tapping a ring shows its burndown.
Until a ring is tapped, the selection is the account Enter would pick. A tapped selection stays until another ring is tapped.
StandBy keeps its own swipes: left and right change screens, up and down flip the stack.
StandBy shows only small widgets, scaled up, so five rings share one row of a small widget. On the Home Screen the layout uses the medium size.
At night StandBy tints widgets red, so the chart must read without its colours: the actual line is solid, the steady line thin and the projection heavy.
To verify on a device: ring tap targets at StandBy scale, tap latency, and that a tap does not wake the full app.

The menu bar icon is Burns' flame (see "Icons"). It takes the pace colour of the account Enter would pick.
Proposal: when the window opens, the ring selected is the account Enter would pick, so the chart answers "how long will my next session's account last". A click selects another ring until the window closes.

#### Reading history (Viz, 2026-10-29; built the same day)

The solid line shows when the quota was used, so `smithersd` keeps every change of each window in a `reading` table.

| Rule | Detail |
|---|---|
| Recorded | A reported live window whose percent differs from the account's last reading of that window, or whose reset moved by more than a minute (a new window). Newer than the last reading only. |
| Not recorded | Repeats. The Claude statusline posts the same percent on every render. |
| Kept | Until the window resets, or until a new window replaces it: Codex can restart a week before the old reset. Then the next window starts empty. |
| Size | Readings change in whole percents, so a window holds about 100 rows. |
| Served | `GET /usage/readings`: every reading of the live windows, oldest first |
| Merging hosts | The union of every host's readings for an account, ordered by observation time |

Readings start when the table does. Claude has no earlier source. Codex rollouts carry their rate limits, but `smithersd` has already read them, and re-reading is not built.

### Merging hosts (proposal, 2026-10-28)

Each `smithersd` knows only its own host. Every reader merges:

1. Quota is shared: it belongs to the account, so any awake host can report it. The newest live reading per account and window wins, and within one window the highest (rules 6 and 10). Readings are never added.
2. Spend is not shared: tokens and cost belong to the host that did the work. Sum every host's answer.

Both apps use one shared Swift package for the merge, ported from Go `quota.Merge` and tested against the same cases.
The iPhone reads every `smithersd` itself, not through the Mac, so it keeps working while the Mac sleeps in a bag.
It keeps each host's last answer with its observation time. A host that cannot be reached shows its cached readings marked with their time; rule 2 still turns them unknown at reset.
A sleeping Mac runs no agents, so its cached spend is still complete, and its cached quota stays accurate for accounts only it uses.

Known limit: usage outside any agent host, such as claude.ai chat or Codex cloud tasks, draws on the same quota but reaches no `smithersd`. A cached reading then reads low until an agent session somewhere reports again.

Burns is written in Go (`smithersd`) and Swift (both apps), matching Roar, Cop, and Penfold.
The Python preview in `burns.py` is superseded.

### Apps and widgets (Viz, 2026-10-29)

Viz wants the widgets built before the OTel cutover. This pulls the usage half of phases 2 and 5 forward; settings for launch, the trigger modal, and launch stay in phase 3.

| Piece | Scope |
|---|---|
| Burns for Mac | Menu bar app with a flame icon. Its window shows the ring row and the selected account's burndown, with refresh, settings and Quit buttons in its top right. A settings window, a grouped form as in System Settings, holds the host list. |
| Mac widget | Small and medium, the same layout as the window |
| Burns for iOS | One screen with the same view, and a settings screen with the host list. No launch. The large title shares the settings button's row (`inlineLarge`). The usage view grows to fill the screen above the host list, at least 320 pt tall; below that the screen scrolls (`ScreenFit`, Viz, 2026-10-30). |
| iPhone widget | Small for StandBy, medium for the Home Screen. Ring taps select an account through an App Intent. |
| `BurnsKit` | Swift package for both apps: the wire types, merge, pace, the Enter pick, the burndown series, the per-host cache, and the views |
| `smithersd` | Tailnet listener for the read routes, with a caller check |

Decisions made while building (2026-10-29, open to Viz's change):

| Decision | Reason |
|---|---|
| Each app keeps its own host list. An entry is a tailnet name such as `host.tailnet.ts.net`. | The phone must work while the Mac sleeps, so it cannot read the Mac's list. |
| The ring row shows every account. Each account name's Claude comes before its Codex, names in settings order, then Azure. Azure draws its full-colour mark with no rings, and selecting it shows "Microsoft credits" in place of a chart. | Azure has no quota windows. An empty track would read as unknown quota; a percent or "unlimited" would be false (rule 9). |
| The burndown charts one window, the longest live one. (Until 2026-10-29 it drew both windows on one axis, which stretched the five-hour line across the week's day grid.) | Viz, 2026-10-29: "I don't think we need the 5h burndown line at all." The ring shows the five-hour window. |
| A widget reads every host itself, at most every 15 minutes, with a 4-second timeout per host. It keeps each host's last answer in the app group. | A widget cannot rely on the app running. A host that does not answer keeps its cached answer, marked with its time. |
| Between reads, the timeline holds an entry every 15 minutes for 2 hours, rebuilt from the cache. | The time arcs and the unknown-at-reset rule move without new data. |
| The selected account lives in the app group. It is shared by every widget on the device and the app's window. | One tap selects the account everywhere on that device. |
| A ring's middle shows the provider's one-colour glyph, tinted: Penfold's vectors in `BurnsKit/Resources`, Claude's spark from Anthropic's press kit and Codex's cloud from the MIT-licensed LobeHub drawing. Azure keeps its own full-colour mark, as in Penfold. | Viz, 2026-10-29: Rex shows the same glyphs, and naming the provider this way is fair use. |
| The Mac app polls every 60 seconds and reloads the widgets only when a host's data changed. | The flame's colour needs fresh quota; widget reloads count against a budget. |
| A widget tap reuses a cache fetched in the last 14 minutes. | The tap answers from the cache, without waiting on the network. |

#### Building the apps (built 2026-10-29)

| Item | Detail |
|---|---|
| Source | `apps/BurnsKit` (Swift package, `swift test`), `apps/Shared`, `apps/Mac`, `apps/iOS`, `apps/Widget`, and the xcodegen spec `apps/project.yml` |
| Signing | `~/.config/burns/signing.env` sets `BURNS_TEAM` and `BURNS_ID_DOMAIN`. It is not committed. The script copies both into `apps/Signing.generated.xcconfig`, which Git ignores, so the Xcode window shows the same names the build uses. Bundle identifiers and app groups are built from the domain. Xcode must be signed in to the team, because the build provisions automatically. |
| Project in Xcode | `scripts/build-apps.sh project` generates the project and opens it. Edits made in Xcode last until the next build regenerates the project. |
| App groups | iOS: `group.<domain>.Burns`, a registered name. macOS: `<team>.<domain>.Burns`, which needs no registration. |
| First iPhone build | The team must have accepted Apple's latest Program License Agreement, or provisioning fails. The entitlements carry literal group names, which xcodegen expands from the environment, so automatic provisioning registers the iOS group itself. The iPhone must be paired with the Mac (Trust This Computer) before the install. |
| Mac | `scripts/build-apps.sh mac` builds Release, installs `/Applications/Burns.app`, and opens it. The widget comes inside the app. |
| iPhone | `scripts/build-apps.sh ios [device]` builds and installs on a connected, unlocked iPhone. |
| Host list from a script | `open -a Burns --args --hosts a.tailnet.ts.net,b.tailnet.ts.net` on the Mac. macOS keeps an app group's preferences from other processes, so a script cannot write them. |
| Looking without a device | `BURNS_RENDER_DIR=<dir> swift test --filter RenderTests` renders the window and both widget sizes, light and dark, for every ring, from saved `<host>-usage.json` and `<host>-usage-readings.json` answers. |

#### Icons (Viz, 2026-10-30; built the same day)

One flame names Burns everywhere: a full-colour 3D flame in the app icons, and its flat outline in the menu bar.

| Surface | Source | Detail |
|---|---|---|
| App icon, Mac and iOS | `apps/Shared/AppIcon.icon` | An Icon Composer icon. Its flame layer, `Assets/Flame.png`, is Viz's ChatGPT-made 3D flame, with stray background specks removed. Under it, `Assets/FlameShadow.png` is a soft shadow: the flame blurred, at half opacity, shifted down. The flame spans about two thirds of the icon's width, as Rex's mark does. The backing is the icon's fill, a subtle diagonal gradient: pale pink to pale cyan in light mode, deep plum to near-black teal in dark. The tinted icon hides the flame and its shadow and shows `Assets/FlameTinted.png` instead: the flame in greyscale with its contrast raised, drawn with the plus-lighter blend mode. Both apps set `ASSETCATALOG_COMPILER_APPICON_NAME` to `AppIcon`. |
| Widgets | Their app's icon | The widget gallery shows the containing app's icon. |
| Menu bar | `Flame` in BurnsKit's `Glyphs.xcassets`, through `BurnsFlame` | The same flame's outline, traced as a flat vector, one colour, as a template image. Amber or red keep the tint when Enter's pick is over pace. |

| Decision | Reason |
|---|---|
| An Icon Composer `.icon`, not the ICNS made with the artwork | iOS takes no ICNS. On macOS 27 the ICNS's own squircle and backing are one bitmap, so the system can make no true dark, tinted or clear icon from it. With the gradient as the fill and the flame as a layer, the system draws every appearance. |
| The gradient is the fill, not an image layer | An image layer stays light in dark mode and turns into a solid slab when tinted. |
| The shadow is its own image layer, not Icon Composer's group shadow | The group shadow draws only under glass layers, and glass frosts the 3D flame's edges. |
| A tinted-only greyscale flame | The tinted icon is drawn from brightness. The flame's colours share one brightness, so they collapse to a muddy purple, and the shadow adds haze. `icon.json` sets `hidden-specializations` and `blend-mode-specializations` for the `tinted` appearance only, so the light and dark icons are unchanged. |
| One 1024 px source, no hand-made sizes | Xcode builds every size from the `.icon`: on macOS, 16 to 512 pt at 1x and 2x, and on iOS, 1024 px for iPhone and iPad plus the 120 px and 152 px home-screen files, each in light, dark and tinted. Both deployment targets are 26, so no older catalog is needed. |
| Our own traced flame in the menu bar, not the SF Symbol `flame.fill` | Apple's terms keep SF Symbols out of app icons, and the menu bar shows the same mark as the icon. Inspired by Streamline's "Matches Fire" icon; no Streamline path is used. |

To look without building: `ictool apps/Shared/AppIcon.icon --export-image --output-file icon.png --platform iOS --rendition Default --width 1024 --height 1024 --scale 1`. `ictool` ships inside Xcode's Icon Composer. Renditions include `Dark`, `TintedDark` and `ClearLight`.

#### Accounts in the apps (agreed, Viz, 2026-10-31; `smithersd` part, glyph colours, account sync and Settings built; Providers screen not built)

Today the apps only read accounts, and only a file edit on each host changes them. But an account is also what Burns launches (phase 3), so the apps must manage it anyway. This design keeps one account list in iCloud, edited in either app, and the apps copy it to every host's `accounts.json`.

> **Core insight: an account is defined once and exists on every host; a host only says whether it is set up there.** The list, its colours, order and launch defaults are central. A host holds a copy, plus what only it can know: its CLIs, and which homes exist on it.

| Decision | Reason |
|---|---|
| The account list lives in iCloud's key-value store (`NSUbiquitousKeyValueStore`), one key per account plus one for the order. An account holds provider, name, kind, home, extra homes, colour, and launch overrides (model, effort, permission mode, and Azure's credential as a cop reference, never the value). Each provider's default model and effort are central too. | Viz, 2026-10-31: accounts are managed centrally. Both apps run under Viz's Apple ID, so iCloud syncs them and no host must stay awake. One key per account means two devices editing different accounts never collide; the same account edited twice ends as the later edit. |
| Every account applies to every host. Homes are relative to the user's home, so one path works on each host. | Viz, 2026-10-31: accounts work on any host by default. |
| The apps propagate the list. On each refresh an app reads each host's accounts with `GET /accounts` and, when they differ from the central list, writes it with `PUT /accounts`. A host that was offline catches up when an app next reaches it. | Hosts never need iCloud. `smithersd` still reads a local file at startup, so a host keeps working with no app running. |
| `PUT` will replace the file's `accounts` only, once the file has `providers` (not built: the file has only `accounts` today, so `PUT` replaces the whole file). The file's `providers`, each provider's CLI on that host, stay as the host set them. | A CLI's path is a fact of one host, not of the account. |
| The central list wins, but never silently. An app remembers the hash it last wrote to each host. If the host's file changed since, by hand or by an agent, the app shows the difference and offers to import it into the central list or overwrite it. A write never deletes fields the app does not edit: an account keeps `launch` and any other field as read. Before each pass, a field that host copies hold and the list's copy lacks joins the list, when every host that has it agrees on its value (2026-11-04). The apps never edit those fields, so a missing one was added on a host, and taking it loses nothing. A host whose copy still holds fields that the list's copy does not match, because hosts disagree, is held for review even when its hash matches. Overwrite in the review is the only write that drops them. | Hand edits and the README setup prompt stay useful, and nothing is lost unseen. |
| On first use, the app imports every host's accounts into the central list, matched by provider and name. If two hosts give one account different homes, the app asks which to keep. | Existing hosts already have files. |
| `PUT` carries `If-Match` with the `ETag` that `GET` returned, a hash of the file. A mismatch answers 412: the app reads again and applies the rule above. | Another device, or a hand edit, may have changed the file between the read and the write. |
| An account with none of its homes on a host, `home` or `extra_homes`, is "not set up" there. `PUT` accepts it, and `GET /usage` lists it with `set_up: false` and no limits. Each scan already skips a missing home, so collecting starts once the home appears, without a restart. | Accounts exist everywhere, so a missing home is expected, not an error. No usage there is unknown, never zero. A launch creates the home while `smithersd` runs. |
| Launching an account on a host where it is not set up sets it up first: Burns creates the home, mode 0700, links its skills, and starts the CLI there. The CLI's own first-run login runs in that session. Settings shows, for each account, the hosts where it is set up. | Viz, 2026-10-31: set up the home on launch. The first launch on a host is when the home is needed, and the login then happens in the terminal Viz is already looking at. |
| `PUT` validates as startup does. A failure answers 422 with the reason, and the file is unchanged. The write is atomic: a temporary file in the same folder, then a rename. Mode 0600. | A bad list never reaches the collector, and a crash mid-write leaves the old file whole. |
| After a write, `smithersd` replaces itself with the same binary and arguments (`exec`). | The collector and the API take the accounts at startup. Re-reading in place would need every part to reload. `exec` launches no other program. |
| Each app copies the central list into its app group for its widgets. Both apps carry the iCloud key-value entitlement with one store identifier under the team. | Widgets read the app group, as they do the host cache. |
| Burns builds the launch command from the provider's CLI on that host and the account's home (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`), not from a stored command line. | Viz, 2026-10-29: Burns replaces the dotfiles launchers. A stored command line would make an editable list a way to run any program. Values become arguments, never shell code. |
| An account name's `colour` is one swatch, shared by its providers. | One colour per account name, as in Penfold: Claude `viz` and Codex `viz` share it. |
| Swatches are fixed pairs, one value for dark backgrounds and a darker one for light, taken in this order: purple, blue, green, pink, teal, grey. No yellow, amber or red. | Penfold's light values were hand-tuned; a free colour picker looks wrong in one mode. Yellow, amber and red are the pace colours. |
| A name with no colour takes the next unused swatch in list order. | Every ring gets a colour without setup. |
| Ring glyphs draw in the account's colour, full strength when selected and at 55% when not. Azure draws its mark in greyscale, keeping its light and dark parts, and in full colour only when selected. | Viz, 2026-10-30, and as Penfold's rings did. Viz, 2026-10-31: greyscale as Rex draws it, not a flat grey silhouette. |
| Ring order is the central order. | One list, one order. |
| Built 2026-10-31: the iCloud list, first-use import and propagation. A host's file is written only when it is what was last written or holds only accounts the list has. Any other difference is held, and Settings says so next to the host. The last written hash is shared in iCloud, so one device's write is not a hand edit on another. An empty list never writes. A `digest` key hashes each saved list, because iCloud delivers keys one at a time. A list with a missing account, or one that does not match the digest, copies to no host, and Settings says "Accounts waiting for iCloud". After 60 quiet seconds a list with every account present is accepted. A missing account stays missing until an edit replaces it. Rings read `colour`. They do not read the central order yet. Found 2026-11-04: `credit_price` had been added to every host's Codex `nfx` entry by hand, and the list lacked it. Every host was then held, so a changed emoji never reached the hosts, and `smithers spawn` kept the old one. Neither review choice was safe: import restored the old emoji, and overwrite dropped the price. Fields the hosts agree on now join the list, so overwrite keeps both. Viz overwrote the three hosts that day: each holds the new emoji and the price. Not verified: that the list crosses devices through iCloud. A new device waits 60 seconds for iCloud before it imports from the hosts. | Writing live configuration must lose nothing unseen. Editing needs Settings, so the crossing is tested there. |
| `colour` and `emoji` are fields in the accounts file, and `smithersd` validates them: a swatch name, and at most 16 runes with no control characters. | Every host must accept what an app sends. A host on an older `smithersd` refuses these fields with 422, and Settings shows that. |
| Later, if needed: an optional `hosts` list per account limits it to those hosts. Not built. | Viz, 2026-10-31: does not need it now. |

Settings on the Mac are toolbar tabs, and the Accounts tab follows Mail: the list beside the selected account, with no navigation inside the window. iPhone settings use one navigation stack.

Screens: Accounts lists names in ring order, each with its swatch and providers. A name opens a page with the swatch row, then each provider entry with its home, extra homes, kind, launch overrides, and the hosts where it is set up so far. Add Account asks for provider, name, home and kind. Providers lists each host's CLIs and each provider's default model and effort.

##### Homes, logins and skills (agreed, not built, Viz, 2026-10-31)

Burns manages homes, logins and skills only when Viz asks it to, and works with the homes and logins that already exist. The Mac app does this work on each host through `rex` and `smithers` subcommands. `smithersd` stays read-only.

| Decision | Reason |
|---|---|
| A typed `~/x` is stored as `x`, relative to the user's home. A file home starting with `~` is refused, because nothing on a host expands it. | The apps write the relative form. |
| Add Account adopts an existing home as it is, or offers a new one, `~/.claude-<name>` or `~/.codex-<name>`. A new home is created on each host at its first launch there. | Nothing moves. On macOS, Claude Code files its login in the Keychain under the home's path, so a moved home loses its login. |
| A new home logs in with the CLI's own login, in the launched session on that host. Each host logs in once per account. | Both CLIs replace refresh tokens as they use them, so a login copied to a second host can log the first one out. Burns never copies login files. |
| Optional for Claude: `claude setup-token` makes one long-lived token, kept in cop and passed at launch with `cop run --env CLAUDE_CODE_OAUTH_TOKEN`. The per-host login stays as the fallback. | cop already replicates its keys to every host. Reports of these tokens failing with 401 mean they cannot be the only login. |
| Skills come from source folders, set per host in order, for example a shared folder, then a Codex-only one. The first source that has a skill name wins for that provider. | Viz's skills live in dotfiles, and some have a Codex version. |
| Burns links each skill into each home that opts in, one symlink per skill. It never replaces a real folder, or a link that points outside the sources. It reports those as conflicts. | Other tools install their own skills in the same folder (cop, roar, the CLIs' own synced and system skills). Burns owns only links into its sources, and removes those when a skill leaves its source. |
| Burns relinks every opted-in home on a host when its sources change, a home is added, or Viz asks. Skill contents reach other hosts through the source folder itself, such as a Git checkout, not through Burns. | Links are per host, so Burns keeps each host's links in step. Copying files between hosts would make two copies of each skill. |
| Antigravity is a later provider, through Gemini API keys only: the key in cop, passed at launch. | Antigravity has no documented per-account home or usage readout, and Burns reads only documented output and the CLIs' own files. |

##### Tool wiring (agreed, not built, Viz, 2026-10-31)

Tools such as roar, smithers, rex and cop each need something in every account's home: a Claude channel flag, an MCP server, a hook. Burns applies what each tool declares, so a new tool is wired the same way without a change to Burns.

> **Core insight: a tool declares what an account needs; Burns applies it at home setup and checks it at launch.** Burns never edits a tool's own entries. It answers only the one consent dialog it caused, and only for the channels it asked for.

| Decision | Reason |
|---|---|
| Each tool's setup on a host writes `~/.config/burns/tools.d/<tool>.json`: per provider, `channel_servers`, `mcp` (name, command, arguments) and `hooks` (event, command, timeout). Commands are absolute paths on that host. | A file is inspectable and needs no program run at launch. The tool's own setup knows its path on that host. |
| Burns applies the declarations when it sets up a home, and checks them at each launch on that host, adding only what is missing. | Idempotent: a new tool reaches existing homes at their next launch. |
| MCP servers register through the CLI's own command, at user scope, in that home: `claude mcp add`, `codex mcp add`. | Burns does not hand-edit the CLIs' configuration files. |
| Hooks are appended to the home's hook list. Burns never reorders or rewrites existing entries. | Codex trusts a hook by its position and hash, so a reorder untrusts it. Other tools' hooks stay as they are. |
| Claude channel servers from every tool merge into one `--dangerously-load-development-channels=server:a server:b` flag at launch. | Claude takes one flag for several servers. |
| Burns answers Claude's development-channels dialog for the sessions it launches. After launch it watches the new pane through Rex for up to 60 seconds. Each capture and key press has its own 10-second limit, so a stalled `rex` cannot keep the watcher past its limits. When the dialog lists exactly the servers Burns put in the flag, Burns selects "I am using this for local development". A dialog that lists anything else is left for the operator, and Burns reports it. Each answer is logged with the pane and the servers. Detection reads the screen with `rex capture -s <session> -b <block> --trim`: the dialog starts "WARNING: Loading development channels" and has a "Channels: server:a ..." line, with `❯` on "1. I am using this for local development". That cursor must be the last one in the capture, with at most 4 painted lines below it: the capture includes scrollback, and a later cursor is Claude's live prompt. A dialog must read as not Burns's for 3 reads in a row before Burns leaves it, because a capture can land mid-paint. This reading of one pane's screen is the one exception to "Burns scrapes nothing" (Viz, 2026-10-31). Burns sends Enter, never Esc, which cancels. Success shows in `roar panes` as path `channel` for that pane. Untested: the Channels line with several servers. | Viz, 2026-10-31: a session launched overnight must not wait on the dialog. On a Pro or Max account a custom channel needs the development flag, and the docs offer no setting that skips its dialog. Matching the exact server list keeps the answer to the channels Burns itself asked for. Roar, 2026-10-31: Claude reports no program status until the dialog is answered, and roar's server has not started, so only the screen shows it. |
| Burns never writes Codex's `trusted_hash`. It reports hooks that still need `/hooks` approval in Settings and in the launched session. | Approval is once per home, not per launch, and an unapproved hook does not stop the session. |
| A Codex launch first runs `CODEX_HOME=<home> codex app-server daemon start`, which does nothing if a daemon runs. It never passes `-c`, `-p`, `--no-daemon` or `--remote`. `-m <model>` and an initial prompt are allowed. | Roar, 2026-10-31, verified on Codex CLI 0.165.0: any `-c` override, even `-c model_reasoning_effort=medium`, runs the TUI in-process, off the home's app-server daemon, so roar cannot deliver natively. `roar panes [host]` shows `codex_native` for a pane on the daemon. |
| Burns launches Codex with no effort setting and never edits a home's `config.toml`. Each home keeps the effort its operator set. Panes launched outside Burns stay as their operator launched them. | Viz, 2026-10-31: Burns cannot force users to change their configuration. `-c` is ruled out above. Roar, 2026-10-31, verified on Codex CLI 0.165.0: `-p` also runs the TUI in-process ("--profile requires embedded mode"). A daemon-backed pane that Burns launches binds at launch (see "Binding a Codex launch to its pane"). Any other daemon-backed pane binds the first time its agent runs a roar command; until then delivery is typed and `roar panes` shows `unbound_codex`. |
| At launch Burns labels the Rex tab with one colour emoji for the account and no name (`rex window rename`). It does not wrap or rename the CLI. The emoji comes from the account name's `emoji`. | Viz, 2026-10-31, tested live: the sidebar row shows the tab label beside the glyph of the foreground program, so `claude` and `codex` keep their icons and roar's and Rex's agent detection is untouched. A colour emoji shows in colour there, and the label held after real Claude and Codex started. Rex has no other colour per session, tab or pane: its icon colours are built into the app per program, and a block label does not show. |
| An account name's `emoji` is one emoji, shared by its providers and central like `colour`. Settings offers a button that opens a searchable grid, the same on Mac and iPhone, with the circles first. It reads Swift's Unicode data, so Burns keeps no list and adds no dependency. It offers single emoji only: no flags or skin tones. With none chosen, the emoji follows the swatch: purple 🟣, blue 🔵, green 🟢, pink 🔴, teal 🟠, grey ⚪. An emoji may repeat across accounts. | Viz, 2026-10-31: pick any emoji per account, and circles by default. Circles have no teal, so teal is the fifth swatch: the first four accounts match, and six accounts still get six emoji. |
| Defaults assume nothing about one operator's setup. With no launch override, an account uses its CLI's own default model, effort and permission mode. Burns adds no tool, skill or folder that a tool or the operator did not declare. | Viz, 2026-10-31: one operator's accounts are one possibility. |
| Burns does not answer Claude's folder-trust dialog. It reports the pane as waiting, and the session waits until the operator accepts. | Viz, 2026-10-31: a launch outside a trusted folder needs Viz's explicit acceptance, so waiting is correct. Seen live, 2026-10-31, in an untrusted folder with no channel flag: `❯` starts on "No, exit", so Enter ends the session. Untested: the order of this dialog and the channels dialog. |
| Burns does not offer a choice of Rex icon. Rex picks a row's icon by the foreground program's exact name, with no API, so a choice would need a wrapper with that program's name, and a wrapper stops roar delivery. | Tested live, 2026-10-31, on real Claude and Codex under a wrapper named `azure`: `roar panes` shows no agent and `refuse:wrong_program` for both, even with Claude's channel connected. A wrapper that stays the process group leader does keep its name and icon. A symlink resolves to its target, and a copied system binary is killed. |
| Burns shows the active panes for each account, from the bindings `smithersd` already stores, and each row names the host, folder and tab. Selecting a row focuses that pane. Untested: whether `rex focus` moves the Rex app, not only the calling client. | Viz, 2026-10-31: with no colour per account, Burns must answer which panes run on which account. |
| At launch Burns sets `BURNS_ACCOUNT=<name>`, and a Claude statusline can print the name. Burns does not edit the statusline command, so each operator adds this to their own script. Untested: that the statusline command inherits the variable. | Claude's statusline is a command, so it can print anything. Sessions not launched by Burns show no name, not a wrong one. |
| Codex's status line does not show the account. | Codex 0.165.0 takes a fixed list of items in `[tui].status_line`, with no custom text and no account item. Only the Rex tab emoji shows the account for Codex. |
| The dotfiles launchers keep wiring until Burns launch is built and verified, then retire. | Viz, 2026-10-29: Burns replaces the launchers after phase 3 ships. |

#### Tailnet listener

| Rule | Detail |
|---|---|
| Address | The first tailnet IPv4 address from `tailscale ip -4`, port 9766. If Tailscale is down at start, `smithersd` retries every 30 seconds. |
| Routes | `GET /usage`, `/usage/readings`, `/usage/history`, `/usage/sessions`, `/usage/panes`, `GET /models`, and `GET` and `PUT /accounts`, so either app can propagate the account list (see "Accounts in the apps"). Every other route answers 404 there. |
| Caller check | `tailscale whois --json` on the caller's address. The caller's login must equal the login that owns this host's node. A tagged node, another user, or a failed lookup with no earlier answer answers 403. This node's own address needs no lookup: `smithersd` learned it and its owner at startup. |
| Cache | Each answer, allowed or refused, is kept 5 minutes per caller address. Requests from one address that arrive together share one lookup. A failed lookup is not kept, and the next request tries again. Until one succeeds, the last answer for that address stands for up to an hour past its 5 minutes. A lookup is limited to 5 seconds. 2026-11-02: at load 57 on 18 cores, `whois` took more than 5 seconds. The Mac app reads its own host over the tailnet and waits 4 seconds, so it reported "timed out". Each concurrent read started its own `whois`, six were killed in one second, and each failure answered 403 with nothing kept. A tailnet address names one node, so a slow `tailscaled` is no reason to refuse a caller allowed minutes earlier. |
| Transport | Plain HTTP inside the tailnet, which WireGuard encrypts. The iOS app allows plain HTTP only for `ts.net` names. |
| Flag | `--tailscale` names the CLI. The install passes the path it finds, as it does for `rex`. Empty disables the listener. |

### Live pane shares (Viz, 2026-11-01; built the same day)

The Mac menu bar window and the iPhone app show, under the burndown, how the selected account's work divides across its live panes. Viz reviewed live mockups and chose this over a list of every session grouped by account, provider, model and version.

| Decision | Why |
|---|---|
| Live panes only, for the selected ring. | Viz, 2026-11-01. A week of sessions is about 250 rows on one host. |
| A segmented bar in the account's colour, stepped by rank, over one line per pane. Each line has the shade, the host's icon, the pane's name, its main model and version, and its percent. There is no token table. | Viz, 2026-11-01. A donut was mocked and not chosen; lopsided shares and close values read better on a bar. Separate hues failed the colour-blind check on the dark surface. The lines name every pane, so shade alone never identifies one. The sixth pane on draws grey. |
| A share uses input, cache writes and output, and leaves out cached input. | A long session rereads its context on every request. With cached input, one Claude pane read 62%; without it, 41% and second. This is a proxy: tokens do not show what a pane cost in quota. |
| A pane appears once with its whole share, subagents included, and shows the model with the most tokens in its own thread, then its count of subagents, such as `Fable 5.1 +7 agents`. Hovering the line shows what the subagents used per model. | Listed under each model, a pane's share split and its true share appeared nowhere. Viz, 2026-11-01: a Fable pane read Haiku 5.5, because 7 subagents made 433 Haiku requests and 7.5M tokens against its 1.4M on Fable. |
| Tokens count from when the pane took the session. | A resumed session's earlier work belongs to its earlier pane. |
| Rex decides what is live, and `smithersd` asks it, so neither app needs the `rex` CLI. A row counts only if its block is in Rex's list now. `checked` says the list is usable. `smithersd` lists panes every 15 seconds and keeps the list in memory, so a read costs no Rex calls. It lists them over one Rex control connection that stays up (`rexkit`'s `rexctl`), attached to the whole server, with `session.list` and `session.list_blocks`, so a refresh starts no process. A list older than 2 minutes counts as no answer. | Until 2026-11-03 each refresh ran `rex session ls` and one `rex block ls` per session, and under load those processes were killed. Rex answers `session.list_blocks` only on a connection attached to the session; attaching to the whole server also covers sessions made later (tested). | An open binding can outlive its pane: 3 of 9 on one host on 2026-11-01. Why those bindings stayed open is not known. |
| A host whose `smithersd` or Rex did not answer is named under the bar, and shares leave it out. | Rule 9: unknown is never zero. A partial Rex list counts as no answer, because it would call live panes dead. Rows with `checked` false or missing count as unread. |
| A host's icon is the one picked in Rex, from any Rex entry for that host: its label or its tailnet name. Rex stores none for its own machine, so that shows a laptop if the Mac has a battery and a desktop if not. Any other host with no pick shows a desktop. | Rex keeps a pick on the one entry it was made on. Viz, 2026-11-01: a pick made in Rex's settings landed on a host's tailnet-name entry, not its label. |
| Both apps reload through one path: usage and panes together, each host shown as it answers. The Mac reads when the window opens and on Refresh. The iPhone reads on launch, on return, and on a pull. Neither reads panes on the 60-second poll. A host added since the last read gets its panes at once, including one added while that catch-up runs. A host removed in settings leaves the shares and cost at once, and with no hosts both clear. An open iPhone screen rebuilds from its cache every 60 seconds with no fetch, so a reset passes and the arcs move. | Only an open screen draws them. The reload runs in its own task: SwiftUI cancels the task of a pull to refresh, and a cancelled read looked like every host failing. A cancelled read says nothing about a host, so it does not mark one as failed. |
| A session nested in a pane, such as `claude -p` run by the agent's shell tool, counts as one of that pane's subagents. | Its bindings share a foreground process and both stay open. The oldest open binding in a block is the pane's own session. |
| The window ends with one line of hosts: each host's icon, name and answer age. A failed host shows a warning symbol, the reason and the age of its data. With too many hosts for the line, ages go first. | Viz, 2026-11-01: one line under the shares, using the same icons. |

Hovering a segment or a line marks both, and dims the other segments. The list does not select or focus a pane. The widgets do not show shares.

On iPhone a tap replaces hovering, and also shows what a pane's subagents used. A phone cannot ask Rex for host icons, so the Mac stores them in iCloud under `burns.hostIcons`. A phone with no such value shows the default desktop.

## Boundaries

| Owner | Owns |
|---|---|
| Rex | Hosts, sessions, tabs, panes, and terminal transport |
| Roar | Message delivery |
| Cop | Credentials |
| Dotfiles | Agent launch configuration, through the launcher functions |
| Burns | Usage visibility, launch choice, and launch request |

Burns takes over usage from Penfold (Viz, 2026-10-27). Penfold's usage code retires after Burns ships.
The switch is a single cutover, not a period of running both (Viz, 2026-10-27).
Only the Claude telemetry endpoint needs the cutover: Claude sends OTLP to one endpoint.
The cutover happened on 2026-10-29 (Viz): the dotfiles `claude` launcher sends OTLP to `http://localhost:9766` on every host. Sessions started before it keep sending to Penfold until they restart.
The statusline forwarder and the SessionStart hook are additive. Claude runs every configured hook, and the statusline script already fans its input out, so `smithers` runs beside Penfold before the cutover.
On 2026-10-28 Viz's statusline script gained the `smithers statusline` forwarder, guarded so that hosts without `smithers` skip it.
`smithersd` does not forward data to Penfold.
Burns must not require Penfold.

## Usage model

### Accounts

An account is a provider plus an account name, such as Claude `viz` or Codex `azure`.
Azure exists only for Codex. Local configuration declares the accounts, their agent homes, and their launchers.
Shared source uses example names only.

`smithersd` reads `$XDG_CONFIG_HOME/burns/accounts.json`, or `~/.config/burns/accounts.json`. `smithers launch` reads the same folder. The example is `docs/accounts.example.json`.

| Field | Meaning |
|---|---|
| `provider` | `claude` or `codex` |
| `name` | Lowercase letters, digits, and hyphens |
| `home` | Agent home, absolute or relative to the user's home, such as `.codex-work` |
| `extra_homes` | Optional. Other homes signed in to the same account, such as the CLI's default `.claude` or `.codex`. They share one quota, so they are one account. Each home belongs to one account only. |
| `kind` | `subscription`, or `azure` (Codex only) |

Unknown fields are errors. File order is the settings order used for ties.
A statusline post or file belongs to the account one of whose homes contains its path, matched as a whole path component.

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
6. When merging hosts, take the newest live reading per `limit_id` and window. Within one window the highest percent wins (rule 10). Keep source times unchanged. Each pool merges on its own; for each window length, the pool with the highest percent stands for the account, and the burndown reads only that pool's readings.
7. Never infer a percentage or a reset time from token counts.
8. A burn rate needs two samples from one session. Until then it is unavailable.
9. Unknown or stale data never displays as zero or unlimited.
10. Usage in one window never goes down. A lower percent for the same reset (within one minute) is stale: an idle session repeats the value of its last response. It is not stored, recorded as a reading, or merged. A new window may start lower.

### Sources

Research on 2026-10-27 and 2026-10-28 picked one live source and one durable backfill per provider.
Every source is a documented feature or a local file the CLI writes. Burns scrapes nothing, with two exceptions: a launch reads its own pane through `rex capture` to answer Claude's channels dialog (see "Launch"), and `smithers spawn` reads the pane it opened for 5 seconds to see Claude's folder-trust dialog (see "Agent-started launches").

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

An OTel event names no account (Viz, 2026-10-29). `smithersd` takes it from the session: the session's transcript sits in one Claude home, and that home is the account.

| Case | Account |
|---|---|
| The session already has a stored row with an account | That account |
| The transcript has not been read yet | None: the request is unattributed. When the transcript's rows arrive, every unattributed row of that session gets their account. |

The settings are then the same in every Claude home. There is no per-home label to keep correct.

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
   `session_meta.cli_version` decides: a rollout from 0.153.4 or later never falls back, even before its first record.
4. Skip a malformed line. Never set a counter to zero.

`smithersd` walks each Codex `sessions` tree and each Claude `projects` tree every 2 seconds while files are changing, and every 30 seconds after 2 minutes without a change. A resumed session appends to its file in the old date folder.
It keeps every cursor in memory, so an unchanged file costs one `stat` and no database read. It skips a file untouched for 7 days.
Decoding an old file still advances decoder state, but rows older than 7 days are not stored.
Before decoding, it skips lines that carry none of the types it reads. Most rollout and transcript bytes are message content.
It reads only complete lines from the stored offset, at most 32 MiB per read, and skips a line longer than 4 MiB.
It stores path, inode, offset, a hash of the first 256 bytes, a hash of the 256 bytes before the offset, and the decoder state. If a hash or the inode stops matching, or the file shrinks, it re-reads from 0.
Do not use `fsnotify`: on macOS it is not recursive.
On 2026-10-28, after the performance fixes, a fresh store backfilled Stiletto's homes in about 5 seconds: 41,944 rows, 104 MB peak memory, then 0.1 percent CPU at idle.

File watching (`fsnotify`) was considered and deferred. On macOS it uses kqueue, which opens a descriptor for every watched file, and missed events still need a reconciling scan. The adaptive scan already removes most idle wakeups. Measure wakeups and energy before replacing it.

#### Compactions

Burns counts every compaction and the tokens the compaction request itself used.
Both CLIs record compactions in local data, so no extra source is needed.

| | Claude | Codex |
|---|---|---|
| Live event | OTel `compaction` event: `session.id`, `prompt.id`, `trigger` (`auto` or `manual`), `success`, `duration_ms`, `pre_tokens`, `post_tokens` | Rollout `compacted` line |
| Compaction tokens | The `api_request` event with `query_source=compact` | The `token_usage_record` whose `response_id` equals the `compacted` line's `compaction_response_id` |
| Backfill | Transcript `system` line with `subtype=compact_boundary`: `compactMetadata.trigger`, `preTokens`, `postTokens`, `durationMs`. No tokens for the request itself. | Same rollout file |
| Dedupe key | None shared. The OTel event and the transcript boundary of one compaction match when they are in one session and within 5 seconds. The OTel copy wins. | `compaction_response_id` |
| Trigger | `auto` or `manual` | Not recorded. Burns shows "unknown". |
| Context before and after | `pre_tokens` and `post_tokens` | Not recorded. The compaction request's `input_tokens` is the context sent to compaction. |

Evidence on 2026-10-28:
- A manual `/compact` on a small Haiku session produced one `compaction` event and one `query_source=compact` request in OTel. The transcript had the boundary but not the request.
- 150 recent Claude transcripts held 78 `compact_boundary` lines, all `auto`.
- Codex rollouts with a `compacted` line: 126 in personal, 109 in azure, 467 lines in nonfx.

The two Claude sources can disagree on context size. One test compaction reported `pre_tokens` 35,988 in OTel and `preTokens` 66,481 in the transcript.

A compaction belongs to the turn it ran in. Its tokens count toward that turn, its session, its block, and its account like any other request.
`GET /usage/sessions` returns each session's compaction count and the compaction share of its tokens. `GET /usage/panes` returns each live pane's count since the pane took its session, and the time of the last one, and `smithers panes` prints them (built 2026-11-02). Neither app shows compactions yet.

Until 2026-11-02 the count was stored and returned only by `/usage/sessions`, keyed by agent session with no pane name, while this section said Burns showed it. Answering how often each lane compacted took a direct query on each host's store.

#### Binding a session to a Rex block

Rex calls a pane a block. Every process in a block inherits `REX_SESSION` and `REX_BLOCK`.
Block IDs are UUIDv7 and scoped to one Rex server, so a binding key is `(host, block)`.
A block can host many agent sessions over time, and a resumed session can move to another block.
So a binding is an interval: host, block, provider, agent session, start, end.

| Path | Provider | How |
|---|---|---|
| Resource attribute | Claude | Inside Rex, the dotfiles `claude` launcher sets `OTEL_RESOURCE_ATTRIBUTES=rex.block=$REX_BLOCK,rex.session=$REX_SESSION` (since 2026-10-29). Every `api_request` names its block. Verified on the wire 2026-10-28. Without it, a request takes its block from the session's binding. |
| SessionStart hook | Claude and Codex | The hook posts provider, agent session ID, `source`, `REX_SESSION`, `REX_BLOCK`, and its ancestor process IDs (`pids`, parent first, at most 16) to `smithersd`. Claude fires it on startup, resume, clear, compact, and fork. Codex fires it at the first prompt. |
| Check | Both | `smithersd` calls Rex's read-only block `process` method (`rex block call --block B process`). See "Checking a claim". |
| End | Both | One `rex events --all --json` stream per host ends bindings. See "Ending a binding". |

Checking a claim:

| Rex answer | Result |
|---|---|
| The block's foreground pid is one of the hook's `pids` | Stored as verified, with that foreground pid |
| The block has no foreground, or its foreground is not among `pids` | Dropped. The hook's `REX_BLOCK` is stale, for example an agent run by a daemon. The session stays unattributed. |
| Rex fails, times out (2 s), or the claim has no `pids` | Stored as unverified, with no foreground pid |

A nested agent, such as `claude -p` run by an agent's shell tool, passes the check: the outer agent is the foreground and an ancestor. Its tokens belong to that block, so this is correct.

Ending a binding:

| Trigger | Bindings ended |
|---|---|
| A new claim in the block | Open bindings for other sessions in the block, unless both have the same known foreground pid. Same foreground means a nested agent, and both stay open. |
| A new claim with `source` `clear` | Also open bindings for the same provider and foreground pid in the block, because `/clear` replaces the session in the same process |
| A claim for the same session in another block | That session's open binding elsewhere (a resume moved it) |
| `block_closed` | Every open binding in the block |
| `session_destroyed` | Every open binding in the Rex session |
| `terminal.process_changed` | With no foreground: every open binding in the block. Otherwise, open bindings whose known foreground pid is neither the new foreground nor one of its `ancestors`. An editor the agent opens (Claude's Ctrl-G) has the agent as an ancestor, so the binding stays. |
| Start, and each reconnect of the events stream | `smithersd` calls `process` for each block with an open binding. A block Rex reports as not found, or one with no foreground, ends its bindings, so events missed while `smithersd` was down still count. A different foreground is left alone, because `process` gives no ancestors; the next claim in that block ends it. |

If Rex is missing, `smithersd` logs it once and keeps unverified claims. The events stream restarts after 1 s, doubling to at most 30 s.

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

#### Billed spend and list price

Built 2026-11-02, except Azure's billed reading, which needs a credential. The account file's `credit_price` field needs the new `smithersd` on every host, because older ones refuse unknown fields.

Once a window is spent, the chart gives way to money: how much past the plan cost, and what the work was worth. An account has two different numbers, and they must never share a label.

| Number | Meaning | Includes plan usage |
|---|---|---|
| Billed | What a provider says it charged, from that provider's own source. | No |
| List price | Burns's estimate: tokens times the published price. | Yes |

**Billed sources**

| Kind of account | Source | Unit and period | Status |
|---|---|---|---|
| Azure | Azure Cost Management, per resource and meter, read with a read-only credential. | Dollars, per day. Refreshes about every 4 hours. | Documented. Not built: it needs a credential. Until then Azure shows "Not available". |
| Claude subscription | `extra_usage.used_credits` and `monthly_limit` from Claude's usage endpoint, `/api/oauth/usage`. | The account's currency, in minor units, per month. | Undocumented, read live on 2026-11-02. `smithersd` polls every 15 minutes with Claude Code's stored login: the keychain on macOS, `.credentials.json` elsewhere. It never refreshes that login, and a failed read keeps the last reading with its age. |
| Codex with credits | `credits.balance` in the rollout and from the app-server's `account/rateLimits/read`. A drop in balance is credits spent. | Credits, between two readings. | Documented, but sparse: one host's rollouts held 166 balances in 54,748 events, and the live read can return null. Workspace credits may be shared. Tested 2026-11-02: `account/rateLimits/read` and the undocumented `/wham/usage` both return a null balance on a workspace with credits, and `/api/codex/usage` answers 403. The undocumented `/wham/usage/daily-workspace-user-token-usage-breakdown` does report credits, per day, in `premium_usage_values.credit_usage_credits`, and `smithersd` polls it every 15 minutes with Codex's stored login. Its 2026-10-21 and 22 values add up to 963.97, and the balance fell 963.98 across those days. It lags: on 2026-11-02 it showed nothing for the last two days, and the panel says so. When it answers, it replaces spend worked out from balances. A real idle day lists each surface at 0, so a row without `credit_usage_credits` is an unknown shape: the whole report is no reading, and the stored history stays. |
| Codex without credits | None needed. | No charge past the limit. | Shown as no credits. |

Rejected: Anthropic's usage and cost API covers Console organizations, not subscriptions. OpenAI's Costs API covers Platform API spend, and third-party documents say it leaves out ChatGPT Codex credits. Its own page could not be read. Codex Analytics appears to be for Enterprise.

**List price**

| Rule | Detail |
|---|---|
| One method | Burns computes every list price from tokens, so accounts share a basis. A reported `cost_micros` is never mixed in. It stays as the check on the table. |
| Price table | Checked in, with `as_of`, `source_url` and `basis` on every row, and start dates so a change does not reprice earlier work. A script refreshes it. `smithersd` never calls a pricing site. |
| Rates | Input, cached input, cache writes by lifetime, and output, by model. Long context applies per request: over 100,000 tokens on Haiku 5.5, over 272,000 on GPT-6 models. Fast tiers apply through `speed` and `service_tier`. |
| New fields | Claude's `speed` and its 5-minute and 1-hour cache writes. Codex's `service_tier`. |
| Hourly totals | Tokens by account, model, tier and hour, kept 400 days. `smithersd` rebuilds the last 3 hours each minute and all retained hours each hour. Dollars are worked out on read, so correcting a price corrects history. |
| Route | `GET /usage/cost?since=192h` answers each host's hours and billed readings. The apps add hours across hosts and take the newest billed reading per account. They read it with panes, not on the 60-second poll. |
| Unknown | A model with no price reads "No price", and the total reads "at least". A row with no tokens is no work. When no host answered, or there are no hosts, list price, Since limit and tokens read "Not available", never $0.00. When some hosts did not answer, they read "at least". Since limit is a floor only when work since the limit has no price. |

Tested on 2026-11-02 against 17,792 Claude requests with a reported cost, $1,954 in all: the table came 4.5% low with every cache write at the 5-minute rate, and 1.6% low at the 1-hour rate. Opus stayed about 3% low at the 1-hour rate, and Haiku about half of a $4 total on one host. Neither gap is explained.

**Screen**

| Rule | Detail |
|---|---|
| When | The panel shows on a spent window, on Azure, and whenever money was billed in the current reset period, even with room left in the week: the five-hour limit can start paid usage (Viz, 2026-11-02). Only a spent window squashes the chart, to about a quarter of its height. Azure has no chart. |
| Order | One large number leads: money billed in this reset period if there was any, else the list price, labelled as that, with tokens below it. Then the month's billed total, "Since limit" and list price by model. |
| Billed this window | Providers report a month's total, so `smithersd` keeps each change, and the apps add what the total rose by inside the window. A fall is a new month. Spend before the history began is left out. |
| Since limit | List price of requests after any window reached 100%, while it stays there. Past five-hour limits are not kept, so this covers only the current one. It is an estimate of what past-limit work cost, and it leaves out plan usage. |
| Missing | A billed number that cannot be read shows "Not available", never $0. A host that did not answer is named. |
| Age | `smithersd` keeps the last billed reading when a poll fails, so the billed line says when it was read: "Read 2h ago". A reading from an earlier month is labelled "Billed in October", with the year when it differs, never "this month". Codex adds "Reported daily, and can lag about two days." |
| Calibration | Where billed and list price cover the same period, Burns shows the difference. |

Decisions (Viz, 2026-11-02):

1. Burns tries the undocumented Claude endpoint.
2. Azure prices as Global Standard.
3. A Codex credit costs what the account paid: the billing page showed 2,052 credits as SGD 106.72, about SGD 0.052 each. The price and its currency are account configuration, because purchase prices differ by plan. Burns converts no currency.
4. Dollars are the headline number, with tokens below them and less prominent.

Auto-reload tops up a Codex balance, so only a fall in balance counts as spend. A rise is a reload, and it is never negative spend.

### Storage

Each `smithersd` keeps one SQLite file in WAL mode, using `modernc.org/sqlite` (pure Go, no cgo).

| Table | Holds | Kept for |
|---|---|---|
| `latest` | Newest reading per account, `limit_id`, and window, with observation time. Claude windows use `limit_id` `claude`. | Until replaced |
| `credits` | Newest Codex credits reading per account and `limit_id` | Until replaced |
| `reading` | Each change of a quota window: account, `limit_id`, window, percent, reset, observation time | Until the window resets |
| `request` | One row per Claude request or Codex response: dedupe key, account, agent session, turn, subagent, compaction flag, tokens, cost | 7 days |
| `compaction` | One row per compaction: account, agent session, turn, trigger, context before and after, duration, request key | 7 days |
| `binding` | Host, block, provider, agent session, start, end | 7 days after end |
| `cursor` | File path, inode, offset, size, head and tail hashes, decoder state | While the file exists |

Per-session, per-turn, per-block, and per-minute views are queries over `request` joined to `binding`.

One connection writes. A pool of 4 read-only connections serves queries, so WAL lets reads run during a write.
Writes are skipped when nothing changed: a repeated request or compaction, and a batch of quota reports that folds to the stored windows. A batch of reports for one limit is folded in memory and written once.
Compaction lookups by `request_key` and by `key` use exact indexes, checked by a query-plan test.
`smithersd` sweeps at startup and then hourly. `since` on read routes is clamped to 7 days.
The planned `minute` and `hour` tables were dropped during phase 1: a week of requests is about 50,000 rows, and indexed queries over them are fast enough.
The file is created owner-only (mode 600) at `$XDG_STATE_HOME/burns/smithers.db`, or `~/.local/state/burns/smithers.db`.
A Codex report older than a stored window never clears that window, so backfill can replay files in any order.

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

The launch reads as a sentence, in this order (Viz, 2026-10-31):

1. Agent: Codex, Claude, Antigravity or Devin.
2. Model: "Default" never shows. Burns resolves it to the actual model, version and effort the account's home would run, and marks that row Default. An account's `launch` override comes first. Then the home's settings. For Codex with neither, the listed model of lowest priority in `models_cache.json`, and its effort from `default_reasoning_level`. A custom provider has no such default.
3. Effort: its own word, shown only when Burns can set it at launch. Codex has none: the model word shows the home's effort, fixed.
4. Billing account: the accounts of that agent, with quota left.
5. Project: a folder.
6. Host: where to run it, listing the hosts that have that project.

A host has an optional display name, set in Settings, for example ColonelK. With none set, Burns shows the Rex label unchanged and never guesses capitals. The display name is for showing only: commands keep the Rex label (Viz, 2026-10-31).

Proposal: the first word comes from the default rule's winning pair. An untouched model follows the billing account's default. Antigravity and Devin are not launchable yet: Burns has no account or usage reading for them.

Model lists stay current from each CLI, not from a list in Burns (Viz, 2026-10-31; Codex and Claude built). A new model needs no Burns release, except a new Claude family.

| Agent | Source | Note |
|---|---|---|
| Codex | The home's `models_cache.json`, which Codex refreshes, plus `config.toml` | Per home, so accounts can differ. Hidden models are left out. A custom provider such as Azure lists only the deployments its `config.toml` names. Effort stays the home's: `-c` is ruled out. |
| Claude | The documented `--model` aliases (`fable`, `opus`, `sonnet`, `haiku`), which the CLI points at the latest version, plus the home's `additionalModelOptionsCache` | Claude has no list command. The version shown is the latest seen in that account's requests. Typing a full model id is allowed. Effort changes with the left and right arrows. |
| Antigravity | `agy models` | Not launchable yet. |
| Devin | `devin models list` | Not launchable yet. |

Each provider has a default model and effort. An account can override them.
Azure needs its own, because its models are deployment names, such as `gpt-6-astra`.
Settings edit both levels.

### Default rule

Quota differs for each provider and account pair, so Enter ranks pairs, not providers (Viz, 2026-10-27).

1. List every quota account: each Claude and Codex account except Azure.
2. For each one, quota left is 100 minus the highest percent used across its live windows.
3. An account with no live window does not compete. A window whose reset passed with no newer reading is left out of the sum, so an idle account can still win (Viz, 2026-10-31). It still shows as unknown, never zero.
4. The account with the most quota left wins. A tie goes to the earlier account in the settings order.
5. If no account has quota left, and every quota account is known to be exhausted, choose Azure.
6. If no account has quota left, but some are unknown, choose the default account from settings. The modal says usage is unknown.

Azure has no subscription quota. It runs on Microsoft credits, so it is the fallback when everything else is exhausted.
Exhausted means a live window at 100 percent.
Rule 5 also puts Azure ahead of paid Claude extra usage.

The winning account brings its model and effort defaults.
With no quota account at all, none has quota left, so the rule chooses Azure.

The modal shows why it picked that account, for example "Codex nfx: 62% left", "All quotas exhausted: Azure", "No quota accounts: Azure" or "Usage unknown: Codex nfx". The line shows while that account is selected.

### Keyboard

| Key | Action |
|---|---|
| Return | Confirm the selection and move to the next part of the sentence. On the last part, launch. (Viz, 2026-10-31) |
| Cmd+Return | Launch what is selected now, from any part of the sentence. (Viz, 2026-10-31) |
| 1, 2, 3 | Choose an option on the current step |
| Tab and Shift+Tab | Move to the next or previous word, wrapping at the ends. While a folder is typed in the project word, Tab completes it instead. |
| Right and Left arrows | Move to the next or previous word, stopping at the ends |
| H | Choose the host |
| S | Choose the session |
| Esc | Step back, or close at the first step |

The final key map needs a visual trial.

### Marks

The Mac app uses Penfold's SVG marks for Claude, Codex, and Azure.
The Unicode marks are only for a terminal fallback, if one is built.

## Launch

Built 2026-10-31, tested live with real Claude on one host: `smithers launch --provider P --account NAME [--model M] [--effort E] [--permission-mode P]`. The app starts it as the command of a new Rex tab, so it runs inside the tab and knows its block. It reads the accounts file and `tools.d`, creates a missing home (mode 0700), sets the environment, starts Codex's daemon, leaves a detached watcher for Claude's channels dialog, and replaces itself with the CLI (`exec`), so no wrapper stays in the pane. Telemetry variables move here from the dotfiles `claude` function. The watcher runs on the host, so a sleeping Mac does not stop it. It keeps waiting while the folder-trust dialog shows, because the channels dialog may follow it. Answers, and a watcher that cannot find `rex`, go to `launch.log` in `$XDG_STATE_HOME/burns` or `~/.local/state/burns`. A failure holds the tab open with the reason. A Claude home at the default `~/.claude` gets no `CLAUDE_CONFIG_DIR`, and an inherited one is removed, because its Keychain login is filed without that path. Accounts take `launch` in the file: `model`, `effort`, `permission_mode` and `credential` (cop `account`, `ref`, `env`). Codex accounts refuse effort and permission mode. Several channel servers pass as one flag with separate values, last on the command line; untested with more than one. Not built: applying `mcp` and `hooks` declarations and skill links.

Built 2026-10-31 for Mac: the panel opens from the menu window's plus button or `burns://new`. It opens at once with the store's current ranking and the last read's projects, because a read takes seconds. Opening the menu window starts a read, and so does opening the panel. A read asks every host at the same time, with a limit of 4 seconds on `GET /models` and 5 seconds on each Rex command. A host that does not answer keeps its last answer. An answer that arrives after the panel opens changes the lists and keeps each word the operator chose or confirmed with Return, if it still exists. Every other word takes the new answer's default. Each host's `GET /models` gives the models, and `rex block call ... process` gives each Rex session's folder: the first pane that answers names it. `pwd` was used until 2026-11-03, but it answers only for a shell that reports its folder, so every session on one remote host and all agent panes came back empty and the panel offered no remote host. The panel then starts `smithers launch` with `rex window new`. A host is offered for a project only when one of its sessions sits in that folder, or, for this Mac, when the folder exists; that last case uses `rex new`. The host list has one row per host and session, with the session's name, because several sessions often share one checkout on one host (seven on one remote host on 2026-11-03). Only this Mac runs without `-S`. A host with no Rex entry is left out, and the panel names it with the reason. A new tab's label is the account emoji alone. The panel reports only whether Rex took the command. Not built: showing the new session, and Confirmed or Failed reporting.

Built 2026-11-03: the New Agent Session Rex action, `burns_new`, in `scripts/rex/new-agent-session.lua`. The Mac's `init.lua` loads the file with `dofile` and binds a key; Viz's Mac binds Shift+Cmd+A, which Rex for Mac leaves free. The action looks for `ctx.session_id` in `session.list` on its own server, then on each server in `rex.servers()`, and runs `open "burns://new?host=<host>&session=<session>"`. The host is empty for the action's own server. Without a session it opens `burns://new`. The panel starts on that host and session once a read lists it, and the target then counts as the operator's choice. A project or host the operator picks first wins. `scripts/check.sh` runs the action's test under `rex do`, because Rex is the only Lua it runs under. Tested live 2026-11-03: the lookup found a remote host's session through `rex.server`, and the action opened the panel from this Mac's session. Pressing the key in a remote host's tab, and the panel's look, wait for Viz.

#### Beside, typed folders and new sessions (Viz, 2026-11-03; built)

Viz, 2026-11-03: "I should be able to launch beside with a different shortcut that will use the currently selected location as the starting point. The selection should allow me to also type out either a path (that should autocomplete) or a session (to create a new one)."

| Part | Rule | Why |
|---|---|---|
| Beside key | A second action, `burns_beside`, on Alt+Shift+Cmd+A, which Rex for Mac leaves free. It opens `burns://new?host=H&session=S&block=B` with the focused block. | A split needs the block it splits. |
| Starting point | The panel reads that block's folder with `process`. The project word starts on that folder, and the host word's first row is "beside this pane", chosen. | Viz: the selected location is the starting point. |
| Beside launch | `rex [-S H] split -s S -b B -c FOLDER --label EMOJI -- smithers launch ...`. The new pane takes focus, as an operator's own split does. | `-c` sets the folder, so `smithers launch` needs no `--dir`. |
| Leaving beside | Any other host row is an ordinary new tab or session. A typed folder on the pane's host keeps the beside row. | The starting point is a default, not a lock. |
| Typed folder | In the project word, typing lists matching projects and the folders that exist on each host, read from `smithersd`'s `GET /dirs`. Tab completes the selected folder and adds `/`. A folder is relative to the home folder; `~/` and a home prefix are stripped. Folders outside the home folder are not offered. | Projects are home-relative, so one typed folder covers every host. |
| A folder's hosts | A typed folder offers each host that has it, with "new session". | The host's `smithersd` saw the folder there. |
| Typed session | In the host word, a typed name that no session on that host has adds "new session NAME" for each host the project lists. Its launch is `rex new NAME`. | Viz: a typed session creates a new one. |
| Letters | Typing keeps its case, and takes `/`, `_` and `~` too, because folders and session names are typed. | |
| Waiting target | While the session or pane the panel opened for is not in the lists, Return launches nothing and the panel says "Finding S…", then "Cannot find S. Choose a project or host." once a read has missed it. Choosing a project or host drops the target. | Peer review, 2026-11-04: without this, Cmd+Return before the read, or after a pane that could not be read, opened an ordinary tab, possibly on another host. |
| Typing and Backspace | The typed session stays pending while its name is typed, and joins the host's rows only when the operator leaves the word on it. Backspace picks again, as typing does, and erasing the whole name returns to the default row. Typing that matches nothing launches nothing: "Nothing matches “Q”." | Peer review, 2026-11-04: typing Fresh then Backspace showed Fres but created Fresh, and every prefix stayed as a row. |
| Beside models | The beside row uses the model catalog of the pane's own host. | Peer review, 2026-11-04: it used the account-wide catalog, which comes from the first host that answers. |
| `GET /dirs?prefix=P` | Read-only. Answers `{"host","home","dirs"}`: up to 50 folders under the home folder whose last part starts with P's last part, sorted, with no case. Hidden folders only when that part starts with `.`. A P that is absolute or has `..` is refused with 400. | The panel can reach a remote filesystem only through `smithersd`. It lists names, never contents. A link whose real path leaves the home folder is never read or listed (peer review, 2026-11-04). |

Built 2026-11-03. The panel asks every host of the last read for `GET /dirs` 120 ms after the last key, and a newer key cancels the request. A host whose `smithersd` lacks the route offers no folders. A chosen typed folder or session stays in the lists after the query clears. The beside target lands on the pane's own folder first. Tested: Swift, Go and Lua tests; `GET /dirs` live on this Mac over loopback and the tailnet; `burns_beside` from this Mac's pane named its block. Not yet seen by Viz: the panel itself, and either key pressed in a remote host's tab. `GET /dirs` was deployed to both remote hosts the same day and answered there.


1. The Rex action finds the server that owns `ctx.session_id` by checking `session.list` on the local server and each server in `rex.servers()`. It then runs `open "burns://new?host=<host>&session=<session>"` on the Mac. Built 2026-11-03.
2. The modal collects the choice.
3. The app resolves the model against the account's catalog. An unknown model is refused before launch. Any change of account, project or host drops a chosen model that the new catalog lacks.
4. The app runs `rex -S <host>` to create the tab or session. The command is the account's launcher with the model arguments.
5. The app asks Rex to show the new session.
6. The app reports Launched, Confirmed, or Failed, following Penfold's launch confirmation rules.

Request values become launcher arguments, never shell code.
Model catalogs port from Penfold's `internal/models` with their tests.

### Agent-started launches (built 2026-11-03)

An agent session can start another agent session without the Mac app. Viz, 2026-11-03: "Burns should launch it for the caller."

`smithers spawn --provider P --account NAME --label L --tab T [--host H] [--beside] [--anchor P] [--model M] [--dir D] [--wait 60s]`, run by the agent from its own pane. There is no prompt option: the caller sends the task through roar once spawn prints the address. A prompt after Claude's channel flag could be read as one more channel value.

| Rule | Detail | Why |
|---|---|---|
| Scope | The new pane joins the caller's own Rex session, found by its label from `REX_SESSION`. It can be on any host. | Viz, 2026-11-03: a caller can only spawn panes in its own project or session. Cop's project is the session label, so credentials follow. |
| Missing session | When `--host` has no session with that label, spawn creates one there in the same folder path. The folder must exist on that host. | Viz, 2026-11-03. The Mac panel already does this for a missing local session. |
| Two sessions with one label | Spawn refuses and names both. | Colonelk had two `stance` sessions on 2026-11-02. Picking one would be a guess. |
| Placement | A new tab by default. `--beside` splits the caller's own tab and puts the new block next to it. `--beside` alone works only on the caller's own host. | Viz, 2026-11-03. The caller's block exists only on its own host. |
| Anchor | `--anchor P` splits beside the pane labelled P in the caller's session, on the target host, and implies `--beside`. It works on any host where that session exists. Spawn refuses when no pane or more than one pane has that label, and names the panes it found. Spawn refuses `--tab` with `--beside` or `--anchor`, since neither opens a tab. |  Viz, 2026-11-04: a comm spawning helpers beside a lane's tab landed them in its own tab (issue #1). The pane label is what roar addresses and is unique in a session; a tab label starts with an emoji and can repeat. |
| Who runs it | The `smithers` CLI in the caller's pane. It runs `rex`, and the new pane runs `smithers launch` as the panel's tabs do. | `smithersd` launches nothing. |
| Folder | The new pane runs `smithers launch --dir D`, which changes into D before it starts the agent. A relative D is relative to the home folder of the host it runs on. D defaults to the caller's folder, relative to the caller's home. A missing folder holds the tab open with the reason. | The same project sits under different homes, such as `/Users/<user>` and `/home/<user>`. Rex's `-c` needs an absolute path, and only the target host knows its home. |
| Label | `--label` is required, and spawn refuses a label already used in that session. | Roar finds a pane by label, so the address must name exactly one pane. |
| Tab | A new tab is named with the account's emoji, then `--tab`, such as `🎱 editor-cap`. `--tab` is required unless `--beside` or `--anchor`, which open no tab. | Viz, 2026-11-03: the lanes' tabs carry a name after the emoji. Roar also resolves a window label. |
| Spent accounts | Spawn warns, and still launches, when the account's live window is at 100% or unknown, or when a subscription has no quota windows on this host at all. Azure has none by design and gets no warning. It never refuses on quota. | Viz, 2026-11-03: a reset may be in hand, and an account on credits may be meant to spend. |
| Result | Spawn waits until Rex reports the agent as the new block's foreground process, up to 60 seconds. It prints the host, block, label and roar address, or says what it is waiting on. Exit 0 when the agent runs, 1 when spawn or Rex refused, 3 when the wait ran out, 4 when Claude waits on its folder-trust dialog, 5 when no screen read worked. | A caller must be able to message what it started. Burns reports only "Rex took the command" today. A pane appears in `smithersd` only once it has made a request, so a binding is not a usable signal. |
| Waiting dialogs | Claude's channels dialog is answered as for any launch. A folder-trust dialog waits for the operator. Claude is already the foreground process while it shows, so for 5 seconds after Claude starts, spawn reads the new pane's screen with `rex capture`. If the dialog shows, spawn names the pane, says the operator must answer it, and exits 4. Roar messages wait until then. | As for the panel's launches. Seen live on 2026-11-03: a folder trusted on one host had never been trusted by that account on another, and spawn reported a running agent. Viz, 2026-11-03, allowed spawn to read the new pane's screen for this. |
| Record | Each spawn is logged on the caller's host with the caller's block, the new block, account and model. | Launches cost quota, and nothing else would show who started one. |

Roar, 2026-11-03: a Claude pane gets native roar delivery only with `--dangerously-load-development-channels server:roar`, and `smithers launch` already passes channel servers. A daemon-backed Codex pane has no roar binding until it receives a typed message and runs `roar` itself. Until then it can receive only typed messages and cannot send. So spawn's result for Codex says the pane is typed-only until bound. Roar offered to bind from a thread ID on the command line, which needs Codex started through its daemon socket (`thread/start`, then `codex resume <id>`). That is not part of this proposal.

Spawn uses `rexkit` v0.1.0, a Rex library shared with roar (see the next paragraph). Roar switched to it in 930a9ee.

Tested live on 2026-11-03 from a Claude pane on Stiletto, with Haiku:

| Case | Result |
|---|---|
| New tab in the caller's session | The agent ran in 2.2 seconds. A roar request to the printed address was read, and the agent answered. |
| `--beside` | The new block opened in the caller's own tab. |
| A label already in the session | Refused before Rex ran anything. |
| `--beside` with `--host` | Refused before Rex ran anything. |
| Another host with no session of that label | Spawn created the session on Baroni. That host's `smithers` predated `--dir`, so the tab held open with "flag provided but not defined: -dir", and spawn exited 3 after 60 seconds. With `a0564d9` deployed there, the agent ran in 1.5 seconds. It then stopped at Claude's folder-trust dialog, because that account had never trusted the folder on that host, and a roar request stayed queued. After the screen check was added, the same case exited 4 and named the dialog. |

`--anchor` was tested live on 2026-11-04 from a Claude pane on Stiletto, in a session with two panes in one tab:

| Case | Result |
|---|---|
| `--anchor` naming the other pane | Claude ran in the new pane. Rex's layout tree showed the new split inside the anchor pane's slot, and the caller's pane kept its own. |
| `--anchor` naming no pane | Refused, naming the session and the missing label. |
| `--anchor` with `--tab` | Refused before Rex ran anything, exit 2. |
| `--anchor` with `--host` where the session does not exist | Refused: that host has no session with the caller's label. |

Roar drives Rex through a Go client of its own: Rex's server API over a WebSocket, plus wrappers for the `rex` CLI. It also resolves names such as `label@session` and `host:` addresses. Burns starts a `rex` process for each call, and on 2026-11-02 those calls were killed under load 24 times. A shared module would give Burns roar's addressing and one connection instead of a process per call.

### Binding a Codex launch to its pane (Viz, 2026-11-04; built)

A daemon-backed Codex runs its SessionStart hook inside the shared daemon. So the hook's `REX_BLOCK` names the wrong pane, and `smithersd` drops the claim. Roar has the same gap: it learns the pane only when the thread receives its first typed message.

**`smithers launch` is the one process that runs in the right pane and knows when Codex starts. It watches the daemon for the thread Codex opens.**

| Step | Who | What |
|---|---|---|
| 1 | `smithers launch` | After `codex app-server daemon start`, start a detached watcher. The watcher connects to `<home>/app-server-control/app-server-control.sock` (WebSocket, JSON-RPC) and reports back once it is connected. |
| 2 | `smithers launch` | `exec codex`. Codex keeps the launch's process ID. |
| 3 | watcher | Wait 10 s for `thread/started` with the launch folder as `cwd`, no `parentThreadId`, and `originator` `codex-tui`. A connection that ends before the 10 s binds nothing: a second thread could have started unseen. |
| 4 | watcher | If exactly one thread matches, ask Rex for the pane's foreground. It must be the launch's PID or run under it, as Codex does under cop. Then run `roar bind codex --thread <id> --pid <pid> --codex-home <home>`, and post the same claim to `smithersd` `/bind` with that PID and `source` `launch`. The watcher inherited the pane's `REX_SESSION` and `REX_BLOCK`. Roar's `pid_not_foreground` is retried once after 1 s, and roar's other refusals are logged. |
| 5 | watcher | If no thread or several threads match, bind nothing and write the reason to `launch.log`. The old paths still apply. |
| 6 | watcher | Before roar runs, check every Codex pane on the host that roar has not bound (`roar panes --json`). If one has its foreground in the launch folder, bind nothing: that pane could have started the thread. A pane roar has bound is no rival, because roar refuses its thread with `thread_bound_elsewhere`. |
| 7 | watcher | If roar refuses the thread itself (`thread_bound_elsewhere`, `invalid_thread`) or another refusal, send no claim to `smithersd`, so another pane's tokens never move here. A refusal about the pane, `not_codex_pane` under a wrapper, still sends the claim. Without roar, nothing is bound. |

Burns opens the Codex daemon socket only for this watcher. It sends no turn and creates no thread.

Tested 2026-11-04 on Codex 0.165.0. The TUI banner showed 0.166.0, the version of the daemon's own package:

| Test | Result |
|---|---|
| Client A calls `thread/start`, and client B calls `thread/resume` | -32600 "no rollout found". A thread gets its rollout at its first turn. Roar's first plan, to create the thread and then run `codex resume <id>`, was dropped for this reason. |
| A client that only listens, while another client starts a thread | The listener receives `thread/started`, so the daemon sends it to every client. |
| Real TUI, a personal Codex home, started in a new Rex session | `thread/started` came 2.3 s after launch, before any prompt: `originator` `codex-tui`, `source` `"vscode"`, `threadSource` `user`, no parent. |
| `/new` in that TUI, choosing the current worktree | Only a `thread/started` for the new thread. The old thread stayed loaded and idle, and no notice named it. |
| Closing the session without a prompt | No rollout file was written for either thread. |

`/new` is not covered. Several panes often share one home and one folder, and a `thread/started` does not name its client.

Step 6 is broader than needed: Rex does not say which Codex home a pane uses, so an unbound Codex in another home and the same folder also blocks the binding. That costs a binding, never a wrong one.

Peer review, 2026-11-04 (Codex, of `767bd90`): steps 3, 6 and 7 came from it. Tested live the same day: a launch beside a hand-started Codex in the same folder logged "could be another pane's: unbound Codex pane …", and a launch with no rival still bound.

Built 2026-11-04: `smithers bind-codex`, started by `smithers launch`, with the daemon client in `internal/codexd`. The launch waits at most 3 s for the watcher to connect, and the watcher tells it over fd 3. Every outcome goes to `launch.log`. Roar's side is `roar bind codex`, from roar commit `8a2ad77`.

Tested live on 2026-11-04, on the Mac, with a personal Codex home, with `smithers launch` in a new Rex session. The log read "codex thread …, pid 74130: roar bound block:…; claim sent to smithersd". `roar panes` showed the pane as `codex_native` before any message, and `smithersd` stored the binding as `hook:launch`, verified, with foreground PID 74130. Not yet tested: the remote hosts, which run the older `smithers`.

A credential launch, tested 2026-11-04:

| Case | Result |
|---|---|
| Real `cop run` | Replaces itself with Codex (`syscall.Exec` in cop's `cmd/cop/main.go`), so Codex keeps the launch's PID. This is the same as the plain launch tested live above. |
| A stand-in `cop` that starts Codex as a child, live | Rex named the wrapper as the pane's foreground. Roar refused with `not_codex_pane`, and `roar panes` showed `refuse:wrong_program`. A wrapper stops roar's delivery anyway (see "Launch"). `smithersd` stored the claim as verified, with the wrapper's PID, which is the right pane for the tokens. |
| Codex showed its "Update available" prompt first | The thread started only after someone answered the prompt, past the 10 s window. Nothing was bound, and the log said "no Codex thread started". The window stays at 10 s: a longer one would risk binding another pane's thread in the same folder, and a wrong binding is worse than none. |

## Wire protocol

`smithersd` listens on `127.0.0.1:9766`. Since the cutover (2026-10-29), Claude sends its telemetry here, not to Penfold on `9765`.
`smithersd` also serves the `GET` routes and `/accounts` on the host's tailnet address, port 9766, and checks each caller there (see "Tailnet listener"). Every route stays on loopback. Loopback routes answer 403 to a `Host` that is not `localhost` or a loopback address, and to any request with an `Origin` header. Tailnet routes apply the `Origin` rule.
These checks stop web pages, not local programs. Another user on the host can reach every loopback route, and a program on the host may pass the tailnet check as the host's owner. Burns adds no token for this: Viz accepted the risk on 2026-11-02, because an admin on the same machine can already reach the agents' own homes.

| Route | Caller | Answer |
|---|---|---|
| `POST /v1/logs` | Claude Code OTLP exporter | Stores `api_request` and `compaction` events. A malformed body answers 200, so the exporter does not retry. Limit 4 MiB. |
| `POST /v1/metrics` | Claude Code OTLP exporter | Accepted and dropped |
| `POST /statusline` | `smithers statusline` | Stores Claude windows for the account that owns `transcript_path`. Limit 8 KiB. |
| `POST /bind` | `smithers hook session-start` | Checks and stores a binding: `provider`, `session`, `rex_block` (required), `rex_session`, `source`, `pids` |
| `GET /usage` | Apps | Every configured account in settings order, with its limits. A window whose reset passed shows `percent: null` and `state: "unknown"`. `set_up` is false when none of the account's homes is on this host, and its limits are then empty. |
| `GET /usage/readings` | Apps | Every quota reading of the live windows, oldest first. See "Reading history". |
| `GET /usage/history` | Apps | One-minute token buckets per account for the last 20 minutes |
| `GET /usage/sessions?since=24h` | Apps | Totals per agent session, and per block and session |
| `GET /usage/sessions?provider=&session=` | Apps | Totals per turn of one session |
| `GET /usage/panes` | Apps | Each open pane's tokens per account and model, largest first: `provider`, `account`, `block`, `model`, `tokens`, `main_tokens`, `agents`, `compactions`, `last_compaction`, `session`, `label`, `last`, and `checked` for the whole answer. `compactions` counts the pane's own session (its oldest open binding) since the pane took it, not a nested session's, and repeats on each model row; `last_compaction` is null with none. Answers from memory. See "Live pane shares". |
| `GET /models` | Apps | What each account can run on this host: `default_id`, `default`, `default_effort` and `models`. See "Steps". Reads the CLIs' files and runs nothing. |
| `GET /accounts` | Apps | The accounts file as it is on disk, with `ETag`, a quoted SHA-256 of its bytes |
| `PUT /accounts` | Apps | Replaces the accounts file. Needs `If-Match` with the current `ETag` (428 without, 412 when it changed). A body that would not load answers 422 with the reason. Limit 64 KiB. Writes atomically, mode 0600, onto a symlink's target, answers the new `ETag`, then `smithersd` restarts. |

Totals carry `cost_micros` for requests with a known cost, `null` when none has one, and `unknown_cost` for the count without one.

### The `smithers` helper

Agents call `smithers`. Its agent-side commands never fail the agent. It waits at most 100 ms (statusline) or 500 ms (hook) for `smithersd`, so install it in the background: the statusline script runs it with `&`, and hooks are registered with `async: true`.

| Command | Job |
|---|---|
| `smithers statusline [-- COMMAND ARGS]` | Posts stdin to `smithersd` with a 100 ms timeout, runs the wrapped statusline command with the same stdin, and prints its output |
| `smithers hook session-start --provider claude\|codex` | Posts the hook's `session_id` and `source` with `REX_BLOCK` and `REX_SESSION`. Does nothing outside Rex. Prints nothing and exits 0, because Claude adds SessionStart output to the conversation. |
| `smithers launch --provider P --account NAME` | Runs in a new Rex tab and becomes the CLI. See "Launch". |
| `smithers answer-channels --block B --servers a,b` | The detached watcher `smithers launch` leaves behind. |
| `smithers usage [--since 24h]` | Prints, for each configured account, its quota windows and its requests, tokens and cost over `--since`. Reads `GET /usage` and `GET /usage/sessions`. Unknown quota prints `unknown`; requests with no cost print as an unknown count, never as $0. Rows with no account print as `<provider> unattributed`, and an account no longer in the file prints under its name. A `--since` past retention prints the 7-day window it covers. Exits 1 if `smithersd` does not answer. |
| `smithers panes [--url URL]` | Prints one line per live pane of one `smithersd`: Rex session, label (or block), provider and account, the model its own thread ran most with its subagent count, compactions since the pane took its session, and how long ago the last was. Reads `GET /usage/panes`; `--url` reads another host over the tailnet. A `smithersd` too old to send the count prints `unknown`, never 0. With no answer from Rex it says the panes are unconfirmed. |
| `smithers spawn --provider P --account NAME --label L --tab T [--host H] [--beside] [--anchor P] [--model M] [--dir D] [--wait 60s]` | Starts an agent in the caller's own Rex session. See "Agent-started launches". Exits 0 when the agent runs, 1 on a refusal, 2 on bad arguments, 3 when the pane opened but the agent did not start in time, 4 when Claude waits on its folder-trust dialog, and 5 when the agent runs but no screen read worked, so the dialog is unchecked. Appends each spawn to `spawn.log` in the state folder. |

Both run beside Penfold's forwarder and hooks. Adding them does not wait for the cutover.

### Installing

`make install` builds both binaries and copies them to `~/.local/bin`. On macOS it then writes the login agent `~/Library/LaunchAgents/com.vizkid.burns.smithersd.plist` and restarts it. Run it again to upgrade.

| Item | Value |
|---|---|
| Accounts | `$XDG_CONFIG_HOME/burns/accounts.json`, or `~/.config/burns/accounts.json`. The install does not create it, because it holds personal names. `smithersd` does not start without it. |
| Database | `~/.local/state/burns/smithers.db`, mode 0600. A fresh database backfills the last 7 days of transcripts and rollouts in a few seconds. |
| Log | macOS: `~/Library/Logs/burns/smithersd.log`. Linux: `journalctl --user -u burns-smithersd`. |
| Agent | `RunAtLoad` and `KeepAlive`, `ProcessType` `Background`, restart throttled to 10 s |
| Rex | The install passes `--rex` with the path of `rex` on its own PATH, because launchd's PATH lacks it. Without it, claims stay unverified and bindings stay open. |
| Tailscale | The install passes `--tailscale` with the path of `tailscale` on its own PATH. Without it, `smithersd` serves no tailnet listener and the apps cannot read this host. |

The install does not touch agent configuration. Register the agent side in each home yourself:

| Agent | Setting |
|---|---|
| Claude statusline | Pipe the statusline input to `smithers statusline` in the background, beside the existing statusline command. |
| Claude SessionStart | A command hook `smithers hook session-start --provider claude` with `async: true` |
| Codex SessionStart | A command hook `smithers hook session-start --provider codex` in `hooks.json`. Codex runs a changed hook only after `/hooks` marks it trusted in that home. |

On Linux the install writes the systemd user unit `~/.config/systemd/user/burns-smithersd.service`, enables it, and restarts it. Its log is in `journalctl --user -u burns-smithersd`. Without lingering (`loginctl enable-linger`) it stops at logout, and the install says so. Without a systemd user manager it only copies the binaries.

### Rex action findings (tested 2026-10-27)

| Finding | Evidence |
|---|---|
| Stiletto's action runs on Stiletto, even when started from a Baroni tab. | `hostname` returned `Stiletto.local` for both palette runs. |
| Its commands run in the GUI login session, so `open` works. | `launchctl managername` returned `Aqua`. `open` exited 0. |
| The palette offers Stiletto's actions while a Baroni session is showing. | The second run reported Baroni's session ID. |
| `ctx` gives the session ID but not its server. | The action must look the session up on each server. Rex's Lua reference now lists `ctx.server` too; the action does not rely on it, because its value is undocumented. |
| The requesting client supplies the servers the action can reach. | A palette run listed `Baroni` and `session.list` on Baroni worked. A CLI run, with or without `-C`, listed none. A `rex.host` line in `init.lua` did not change this. |

## Phases

1. `smithersd` collects Claude and Codex usage on one host, and the rules pass their tests. Built 2026-10-28; see "Phase 1 verification".
   1b. `smithersd` checks each hook binding against Rex's block `process` method and ends bindings from `rex events`. Built 2026-10-28; see "Phase 1 verification".
2. The Mac menu bar app merges every host and shows usage. Its usage view and widgets were built 2026-10-29 with Burns for iOS's (see "Apps and widgets"); the tailnet listener too.
3. The trigger modal, settings, and launch through Rex.
4. The New Agent Session Rex action. Built 2026-11-03; see "Launch".
5. Burns for iOS shows usage.
6. Penfold's usage code retires.

## Performance review (2026-10-28)

A Codex peer review of phase 1 found eight issues. Fixed the same day, with tests:

| Finding | Fix |
|---|---|
| Compaction lookups scanned a provider's rows | `compaction_request` index; keyed lookup repeats the partial index condition |
| Backfill rewrote quota on every historical report | Reports fold in memory per batch; unchanged results and unchanged requests are not written |
| One connection served reads and writes | Read-only pool of 4 connections |
| Old events in resumed files entered the store | Rows older than retention are not stored; sweep at startup |
| Every scan read each file's cursor from SQLite | Cursors cached in memory; one `stat` per file |
| Irrelevant lines were fully decoded | Byte markers skip lines before JSON decoding |
| Large `since` values widened queries | Clamped to retention |

Open: the hook and statusline helper wait up to 500 ms and 100 ms, so they must run in the background (see "The `smithers` helper"). Bindings that never close are bounded in phase 1b. Peak memory during a large backfill is 104 MB and not yet tuned.

### Push to the apps (proposal)

The apps should not poll. `GET /usage/events` would stream server-sent events to the Mac app, with these rules from the review:

- Send a coalesced "changed" notice after a committed change, not every source event. Clients then fetch the routes they show.
- Share one snapshot across subscribers. Bound each client's queue and drop slow readers without blocking ingestion.
- Give the stream its own write deadline. `smithersd`'s 30-second `WriteTimeout` would otherwise end it.
- Quota expiry and the rolling history window change without new data. Clients compute those from timestamps.
- The iPhone streams only while the app is open. Widgets refresh on the system's schedule.

## Phase 1 verification

Checked on Stiletto on 2026-10-28 with `make check` green and `smithersd` reading the real agent homes.

| Check | Result |
|---|---|
| Codex totals against an independent count of the same rollouts | Exact match for 3 rollouts: requests, tokens, compactions. One had 4,053 requests and 30 compactions. |
| Claude live: `claude -p` with OTel to `smithersd`, then `/compact` | Cost matched the CLI to the micro-dollar. The compaction counted once, its request was flagged, and the session landed on the pane's Rex block. |
| `smithersd` down for one turn | After restart the transcript added the missing request once. Its cost reads unknown. |
| `smithers statusline` and `smithers hook session-start` | Windows stored, wrapped command output passed through, binding stored. The hook printed nothing. |
| 1b: nested `claude -p` in an agent's pane | Verified, with the outer agent as foreground. The outer binding stayed open. |
| 1b: hook with another pane's `REX_BLOCK` | Dropped: that pane's foreground was not among the hook's ancestors. |
| 1b: a pane that closes | Its binding ended when the pane closed, from the events stream. |
| 1b: a pane that closes while `smithersd` is down | The binding ended at restart, from the reconcile. |
| 1b: block lookup scope | Inside a pane, `REX_SESSION` limits `rex block call` to that session. `smithersd` removes `REX_SESSION` and `REX_BLOCK` before calling `rex`. |
| Readings, 2026-10-29 | After migration 4, the first statusline post stored one reading per Claude window. Later posts at the same percent stored nothing. |
| Account without a label, 2026-10-29 | `claude -p` with OTel to `smithersd` and no `burns.account`. The request arrived unattributed with its cost, then took the personal account when the transcript was read about 30 seconds later. |
| Baroni and Colonelk, 2026-10-29 | Installed from `main`: launchd on Baroni (macOS), systemd user service on Colonelk (Linux). A fresh store backfilled 7 days on each, including Codex readings. Colonelk and Baroni held newer Codex quota than Stiletto: Codex nfx at 100%, which Stiletto showed as 9% from an older reading. |

## Widget verification (2026-10-29)

| Check | Result |
|---|---|
| Tailnet listener on all three hosts | Each logs `tailnet listener on <tailnet IP>:9766` for the owner's login. From Stiletto, `GET /usage` and `/usage/readings` answer 200 on each host's tailnet name, and `POST /bind` answers 404. Loopback is unchanged. |
| Burns for iOS in the simulator, reading the three hosts | All three hosts fresh. Rings for Claude and Codex of both accounts. Enter's pick (the personal Codex account, 81% left) selected at start, with its burndown, steady line and projection. |
| Merge against `smithers usage` | The personal Codex account's 19% and the personal Claude account's seven-day 28% match. The work Codex account shows Baroni and Colonelk's newer 100%, not Stiletto's stale 9%. The work Claude account, unknown on Stiletto, shows the other hosts' reading. |
| Widget layouts | Rendered from the hosts' saved answers in both sizes, light and dark, for every ring. |
| Burns for Mac | Built, signed and installed in `/Applications`. Its window was not captured: this session has no Screen Recording or Accessibility permission. |
| Not yet verified | Burns for iOS on the iPhone: Xcode's sign-in to the Apple account was refused, so the app group cannot be provisioned. On a device: StandBy ring tap targets, tap latency, and that a tap does not wake the app. On the Mac: adding the widget and a ring tap. |

## Open questions

| Question | Status |
|---|---|
| Does the palette offer Stiletto's actions while the app shows a Baroni session? | Yes. Tested 2026-10-27 with a temporary action. |
| Does `open` from a Rex action open the deeplink on Stiletto? | Yes. Tested 2026-10-27. |
| Should the account also be picked by quota left, not only the provider? | Yes: Enter ranks provider and account pairs. Azure is the fallback. (Viz, 2026-10-27) |
| Should Burns track the Azure credit balance? | Probably yes, but deferred (Viz, 2026-10-27). Not in the current phases. |
| Does the iOS app read hosts from synced settings or its own list? | Its own list, in each app's settings (decided 2026-10-29 while building the widgets; see "Apps and widgets") |
| Source license | MIT (Viz, 2026-10-27) |

## Cross-references

- Rex decision record: `rex/decisions.md`, "Burns: picker direction"
- Penfold launch: `docs/specs/LAUNCH_SPEC.md`, sections "Launcher ownership", "Models", "Launch confirmation"
- Penfold quota: `docs/specs/CLAUDE_QUOTA_INGESTION_SPEC.md`, `docs/specs/TOKEN_BURN_SPEC.md`
- Penfold marks: `apps/PenfoldKit/Sources/PenfoldKit/ProviderMark.swift`, `WidgetProvider.swift`
- Penfold rings and pace: `apps/PenfoldKit/Sources/PenfoldKit/SpendTile.swift`, `SpendPace.swift`
- Prior art: CodexBar (github.com/steipete/CodexBar), a menu bar quota meter for Claude and Codex; it reads OAuth and browser cookies, which Burns does not

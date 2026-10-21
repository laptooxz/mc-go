# mc-go

Compiled Go replacement for the legacy `mc` fish function. Manages local
Minecraft servers on `laptoo`.

> **yep this is entirely vibecoded.**

> **Built for one homelab.** This is written for and tested only on `laptoo`
> (Alpine Linux, OpenRC, my own directory layout). It can be adapted to other
> systems — see [Portability](#portability) — but it is not portable as-is,
> and it has no CI, no test suite and no security audit. Read the source before
> running it anywhere that matters.

## Requirements

The binary is static, but most commands shell out to system tools:

| Needed for | Dependency |
|---|---|
| everything | `tmux` (server process manager), `pgrep`, `tar`, `ss`, `pkill` |
| `java` / `setup` pickers | `fzf` — optional; falls back to a numbered prompt |
| `setup … flags` | `$EDITOR` — optional; falls back to a single-line prompt |
| `bot` | `node` + the bot engine in `~/mc/bot/` (`mineflayer`, plus `bedrock-protocol` for `-b`) — **not shipped in this repo** |
| notifications | `notify-send` + `paplay`, and an ntfy token at `/etc/sysmon/tokens/ntfy.sh` — all optional |

The bot engine lives outside this repo (`~/mc/bot/index.js` /
`index-bedrock.js` + `node_modules`); `mc bot` only launches it.

## Usage

```
mc <command> [server]

  start        Start server in tmux
  stop         Stop server gracefully
  restart      Restart server gracefully
  console      Attach to server console
  logs         View logs (tail -f)
  backup       Create backup
  restore      Restore from backup
  status       Check if server is running
  check        Check server files
  rmworld      Delete world folder
  list         List all servers
  watch        Check mc-watcher status
  whitelist    Manage whitelist (list|add <player>|remove <player>)
  bot          Manage offline bots (list|join [username] [-b]|kick <username>)
  java         Configure per-server Java version (fzf picker)
  setup        Unified setup: Java + RAM + JVM flags (fzf menu)
```

### java

```
mc java                # check all + pick server→JVM via fzf
mc java check          # table: .java_version health + suggestions
mc java list           # installed JVMs (/usr/lib/jvm/*/bin/java)
mc java <srv>          # pick JVM for <srv> via fzf
mc java <srv> 21       # set non-interactively (8|17|21|25 or label/path)
```

### setup

```
mc setup               # pick server → interactive menu (fzf)
mc setup <srv>         # menu: Java / RAM / Flags / View / Reset
mc setup <srv> java [jvm]
mc setup <srv> ram [Xmx] [Xms]   # e.g. `mc setup survival ram 6G` or `8G 4G`
mc setup <srv> flags "[flags]"   # raw JVM flags
mc setup check         # table: Xms/Xmx + flags + java health
```

## Build

```sh
go build -o mc .
sudo install -m 755 mc /usr/local/bin/mc
```

## Layout

- `main.go` — CLI dispatch + server discovery helpers
- `commands.go` — per-subcommand implementations
- `exec.go` — tmux orchestration + stop-timeout/retry + tty keyboard prompt
- `notify.go` — desktop notify-send (host GUI env), paplay sound, ntfy push
- `bot.go` — offline bot state/kick integration (`~/mc/bot/.bots/*`)
- `config.go` — Aikar flags + per-server `.java_args` (`readJvmArgs`/`writeJvmArgs`/`parseMem`)
- `java.go` — JVM discovery (`/usr/lib/jvm/*/bin/java`) + `mc java` checker & fzf picker
- `setup.go` — `mc setup` unified menu (Java + RAM + flags, fzf + $EDITOR fallback)

## Portability

Nothing here is configurable at runtime — these assumptions are compiled in:

| Assumption | Where |
|---|---|
| servers live in `~/mc/<server>` (a dir with at least one `.jar`) | `main.go` (`mcBase`) |
| backups are written/read in `/mnt/HDD/mc/` | `commands.go` |
| JVMs are discovered at `/usr/lib/jvm/*/bin/java` | `java.go` |
| bot engine, state and logs live in `~/mc/bot/` | `bot.go`, `commands.go` |
| Minecraft sounds live in `~/mc/library/sounds` | `notify.go` |
| default JVM flags include `-Djava.io.tmpdir=/var/tmp` | `config.go` |
| ntfy token is read from `/etc/sysmon/tokens/ntfy.sh` | `notify.go` |
| tmux sessions are named `mc-<server>` | `commands.go` (`sessionFor`) |

To port it, change those constants and rebuild. `~/.config/mc/config.json` is
reserved by `configPath()` but is not read yet, so there is no config file
today.

## Behavior notes

- Stop sends `stop` to the console, waits 30s, then offers a force-kill.
  Pressing any key jumps straight to the prompt; `y` force-kills. If declined
  it retries the 30s wait up to 3 times, then lists any surviving tmux
  session / java PID for manual handling.
- Jar name and java provider come from `server.properties`/`.java_version`
  using the Aikar flags (was `$mcargs` in the fish version).
- Per-server Java: `~/mc/<srv>/.java_version` stores the java binary path
  (e.g. `/usr/lib/jvm/java-21-openjdk/bin/java`). `mc java` discovers
  installed JVMs via `/usr/lib/jvm/*/bin/java` and shows version + suggestion
  based on jar name (1.7→8, 1.17→17, 26.x→25). Picker is `fzf` with `rounded`
  border (falls back to numbered prompt when no TTY/fzf).
- Per-server RAM/flags: `~/mc/<srv>/.java_args` (also `.mc_args`/`.jvm_args`)
  stores raw JVM flags. Missing → built-in Aikar flags (`-Xms2G -Xmx4G …`).
  `mc setup` edits both `.java_version` and `.java_args` in one fzf menu:
  Java picker, RAM picker (fzf presets 512M→16G + custom), Flags editor
  (`$EDITOR` on temp file or single-line prompt). `View` shows the final
  `java <flags> -jar <jar> --nogui` command.

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
| `bot` | `node` + a bot engine (`mineflayer`, plus `bedrock-protocol` for `-b`) — **not shipped in this repo** |
| notifications | `notify-send` + `paplay`, and an ntfy token file — all optional |

The bot engine lives outside this repo (default `~/mc/bot/index.js` /
`index-bedrock.js` + `node_modules`, see `bot_dir`); `mc bot` only launches it.

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
  config       Show or edit runtime paths (~/.config/mc/config.json)
```

### java

```
mc java                # check all + pick server→JVM via fzf
mc java check          # table: .java_version health + suggestions
mc java list           # installed JVMs (<jvm_root>/*/bin/java)
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
- `bot.go` — offline bot state/kick integration (`<bot_dir>/.bots/*`)
- `config.go` — runtime config (`~/.config/mc/config.json`), Aikar flags, per-server `.java_args`
- `java.go` — JVM discovery (`<jvm_root>/*/bin/java`) + `mc java` checker & fzf picker
- `setup.go` — `mc setup` unified menu (Java + RAM + flags, fzf + $EDITOR fallback)

## Configuration

Paths and a few layout assumptions are read from
`~/.config/mc/config.json`. The file is optional: if it is missing, every
value falls back to the original laptoo layout, so an existing install keeps
working with no config at all.

| Key | Default | Meaning |
|---|---|---|
| `server_root` | `~/mc` | one directory per server (each must contain a `.jar`) |
| `backup_dir` | `/mnt/HDD/mc` | where `backup` writes and `restore` looks for archives |
| `jvm_root` | `/usr/lib/jvm` | scanned as `<jvm_root>/*/bin/java` |
| `bot_dir` | `~/mc/bot` | bot engine (`index.js`), `.bots` state and logs |
| `sounds_dir` | `~/mc/library/sounds` | `.ogg` files used by desktop notifications |
| `tmp_dir` | `/var/tmp` | `-Djava.io.tmpdir` in the built-in default JVM flags |
| `session_prefix` | `mc-` | tmux session name = `<prefix><server>` |
| `ntfy_url` | `http://127.0.0.1:8989` | local ntfy base URL |
| `ntfy_token_file` | `/etc/sysmon/tokens/ntfy.sh` | file containing `NTFY_TOKEN=...` |

Any subset of keys may be set; empty or missing keys use the default. Values
may use `~` and `$VARS`, and paths under `$HOME` are written back as `~/...`.
Manage it without editing JSON by hand:

```
mc config                 # show effective values + whether each came from the file
mc config path            # print the config file path
mc config init [--force]  # write a full default config
mc config set <key> <val> # change one key (rewrites the whole file)
mc config help            # keys + defaults
```

Changing `tmp_dir` only affects the **built-in default** flags. A server with
its own `.java_args` keeps whatever `-Djava.io.tmpdir=` it already has.

## Portability

The defaults reproduce this homelab, but the remaining OS assumptions are
still real:

- `services.sh` in sysmon (a separate repo) is OpenRC-only, and sysmon reads
  Alpine's `/var/log/messages` — that is unrelated to `mc`.
- `mc` expects `tmux`, `pgrep`, `tar`, `ss` and `pkill` on `PATH`.
- Per-server Java is a path in `.java_version`; `java` on `PATH` is the
  fallback when no file exists.
- `mc-watcher` (separate repo) hardcodes `~/mc/<server>` and tmux sessions
  named `mc-<server>`. If you change `server_root` or `session_prefix`, the
  watcher will not see those servers/sessions unless you patch it too.
- Desktop/ntfy notifications degrade quietly when `notify-send`, `paplay` or
  the token file are missing.

Everything in the table above is configurable without a rebuild.

## Behavior notes

- Stop sends `stop` to the console, waits 30s, then offers a force-kill.
  Pressing any key jumps straight to the prompt; `y` force-kills. If declined
  it retries the 30s wait up to 3 times, then lists any surviving tmux
  session / java PID for manual handling.
- Jar name and java provider come from `server.properties`/`.java_version`
  using the Aikar flags (was `$mcargs` in the fish version).
- Per-server Java: `~/mc/<srv>/.java_version` stores the java binary path
  (e.g. `/usr/lib/jvm/java-21-openjdk/bin/java`). `mc java` discovers
  installed JVMs via `<jvm_root>/*/bin/java` and shows version + suggestion
  based on jar name (1.7→8, 1.17→17, 26.x→25). Picker is `fzf` with `rounded`
  border (falls back to numbered prompt when no TTY/fzf).
- Per-server RAM/flags: `~/mc/<srv>/.java_args` (also `.mc_args`/`.jvm_args`)
  stores raw JVM flags. Missing → built-in Aikar flags (`-Xms2G -Xmx4G …`).
  `mc setup` edits both `.java_version` and `.java_args` in one fzf menu:
  Java picker, RAM picker (fzf presets 512M→16G + custom), Flags editor
  (`$EDITOR` on temp file or single-line prompt). `View` shows the final
  `java <flags> -jar <jar> --nogui` command.

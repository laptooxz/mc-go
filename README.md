# mc-go

Compiled Go replacement for the legacy `mc` fish function. Manages local
Minecraft servers on `laptoo` (Alpine Linux, OpenRC, no systemd).

> **yep this is entirely vibecoded.**

Single static binary, no runtime deps beyond `tmux` (the server process
manager) and standard system tools (`pgrep`, `tar`, `ss`, `pkill`).

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

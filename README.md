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
- `config.go` — `mcargs` java-line + future config override

## Behavior notes

- Stop sends `stop` to the console, waits 30s, then offers a force-kill.
  Pressing any key jumps straight to the prompt; `y` force-kills. If declined
  it retries the 30s wait up to 3 times, then lists any surviving tmux
  session / java PID for manual handling.
- Jar name and java provider come from `server.properties`/`.java_version`
  using the Aikar flags (was `$mcargs` in the fish version).

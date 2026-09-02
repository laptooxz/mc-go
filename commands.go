package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func requireServer(args []string) (srv string, err error) {
	if len(args) < 1 {
		return "", fmt.Errorf("Usage: mc %s <server>", os.Args[1])
	}
	srv = args[0]
	if _, err := os.Stat(filepath.Join(mcBase(), srv)); err != nil {
		fmt.Fprintf(os.Stderr, "Server '%s' does not exist.\nAvailable servers:\n", srv)
		list, _ := listServers()
		for _, s := range list {
			fmt.Println("  " + s)
		}
		return "", fmt.Errorf("no such server")
	}
	return srv, nil
}

func sessionFor(srv string) string { return "mc-" + srv }

func runCmd(srv string) string {
	dir := filepath.Join(mcBase(), srv)
	jar, err := findJar(dir)
	if err != nil {
		jar = "paper.jar"
	}
	java := readJavaCmd(dir)
	flags := readJvmArgs(dir)
	// reconstruct: <java> <flags> -jar <jar> --nogui
	return java + " " + flags + " -jar " + jar + " --nogui"
}

// ---------------- list ----------------
func cmdList() error {
	l, err := listServers()
	if err != nil {
		return err
	}
	fmt.Println("Available servers:")
	for _, s := range l {
		fmt.Println("  " + s)
	}
	return nil
}

// ---------------- check ----------------
func cmdCheck(args []string) error {
	srv, err := requireServer(args)
	if err != nil {
		return err
	}
	dir := filepath.Join(mcBase(), srv)
	jar, err := findJar(dir)
	if err != nil {
		fmt.Printf("%s: INVALID (no .jar file)\n", srv)
		return nil
	}
	fmt.Printf("%s: VALID (%s)\n", srv, jar)
	return nil
}

// ---------------- watch ----------------
func cmdWatch() error {
	out, err := exec.Command("sudo", "rc-service", "mc-watcher", "status").CombinedOutput()
	if strings.Contains(string(out), "started") {
		fmt.Println("mc-watcher is running. It auto-detects active servers.")
	} else {
		fmt.Println("mc-watcher is not running. Start with: sudo rc-service mc-watcher start")
	}
	return err
}

// ---------------- start ----------------
func cmdStart(args []string) error {
	srv, err := requireServer(args)
	if err != nil {
		return err
	}
	dir := filepath.Join(mcBase(), srv)
	session := sessionFor(srv)
	jar, err := findJar(dir)
	if err != nil {
		return err
	}

	if tmuxHasSession(session) {
		fmt.Printf("Server %s is already running (tmux session found).\n", srv)
		return nil
	}
	for _, pid := range javaPidsForDir(dir, jar) {
		fmt.Printf("Server %s has a leftover process (PID %s). Killing stale process...\n", srv, pid)
		exec.Command("kill", pid).Run()
		time.Sleep(1 * time.Second)
		exec.Command("kill", "-9", pid).Run()
		time.Sleep(1 * time.Second)
	}

	os.Chdir(dir)
	cmdline := runCmd(srv)
	exec.Command("tmux", "new-session", "-d", "-s", session, cmdline).Run()
	fmt.Printf("Minecraft %s started in tmux session %s\n", srv, session)
	notify(srv, "Server started", "start")
	return nil
}

// ---------------- stop ----------------
func cmdStop(args []string) error {
	srv, err := requireServer(args)
	if err != nil {
		return err
	}
	dir := filepath.Join(mcBase(), srv)
	session := sessionFor(srv)
	jar, err := findJar(dir)
	if err != nil {
		return err
	}

	if !tmuxHasSession(session) {
		fmt.Printf("Server %s is not running (no tmux session).\n", srv)
		return nil
	}

	fmt.Printf("Stopping %s...\n", srv)
	kickBotsForPort(dir)
	tmuxSend(session, "")
	tmuxSend(session, `tellraw @a {"text":"Server Shutting Down...","color":"red","bold":true}`)
	time.Sleep(3 * time.Second)
	tmuxSend(session, "stop")

	waitStop(srv, session, jar, 30, 3)

	if tmuxHasSession(session) {
		exec.Command("tmux", "kill-session", "-t", session).Run()
	}
	fmt.Println("Server stopped.")
	notify(srv, "Server stopped", "stop")
	return nil
}

// ---------------- restart ----------------
func cmdRestart(args []string) error {
	srv, err := requireServer(args)
	if err != nil {
		return err
	}
	dir := filepath.Join(mcBase(), srv)
	session := sessionFor(srv)
	jar, err := findJar(dir)
	if err != nil {
		return err
	}

	if !tmuxHasSession(session) {
		fmt.Printf("Server %s is not running.\n", srv)
		return nil
	}

	fmt.Printf("Restarting %s...\n", srv)
	kickBotsForPort(dir)
	tmuxSend(session, "")
	tmuxSend(session, `tellraw @a {"text":"Server Restarting...","color":"gold","bold":true}`)
	time.Sleep(3 * time.Second)
	tmuxSend(session, "stop")

	waitStop(srv, session, jar, 30, 3)

	if tmuxHasSession(session) {
		exec.Command("tmux", "kill-session", "-t", session).Run()
	}

	fmt.Printf("Starting %s...\n", srv)
	os.Chdir(dir)
	cmdline := runCmd(srv)
	exec.Command("tmux", "new-session", "-d", "-s", session, cmdline).Run()
	fmt.Printf("Minecraft %s restarted in tmux session %s\n", srv, session)
	notify(srv, "Server restarted", "start")
	tmuxAttach(session)
	return nil
}

// ---------------- console ----------------
func cmdConsole(args []string) error {
	srv, err := requireServer(args)
	if err != nil {
		return err
	}
	session := sessionFor(srv)
	if !tmuxHasSession(session) {
		fmt.Printf("Server %s is not running.\n", srv)
		return nil
	}
	tmuxAttach(session)
	return nil
}

// ---------------- logs ----------------
func cmdLogs(args []string) error {
	srv, err := requireServer(args)
	if err != nil {
		return err
	}
	logPath := filepath.Join(mcBase(), srv, "logs", "latest.log")
	if _, err := os.Stat(logPath); err != nil {
		fmt.Printf("No logs found for %s\n", srv)
		return nil
	}
	cmd := exec.Command("tail", "-f", logPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ---------------- backup ----------------
func cmdBackup(args []string) error {
	srv, err := requireServer(args)
	if err != nil {
		return err
	}
	dir := filepath.Join(mcBase(), srv)
	if _, err := os.Stat(filepath.Join(dir, "world")); err != nil {
		fmt.Printf("No world folder found in %s\n", srv)
		return nil
	}
	dest := "/mnt/HDD/mc"
	os.MkdirAll(dest, 0755)
	ts := time.Now().Format("2006-01-02_1504")

	tarDirs := []string{"world"}
	if _, err := os.Stat(filepath.Join(dir, "world_nether")); err == nil {
		tarDirs = append(tarDirs, "world_nether")
	}
	if _, err := os.Stat(filepath.Join(dir, "world_the_end")); err == nil {
		tarDirs = append(tarDirs, "world_the_end")
	}

	start := time.Now()
	argsT := append([]string{"-czvf", filepath.Join(dest, srv+"-"+ts+".tar.gz"), "-C", dir}, tarDirs...)
	cmd := exec.Command("tar", argsT...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	dur := time.Since(start)
	durStr := formatDuration(int(dur.Seconds()))
	fmt.Printf("Backup created: %s/%s-%s.tar.gz (%s)\n", dest, srv, ts, durStr)
	notify(srv, "Backup finished in "+durStr, "window-attention")
	return nil
}

func formatDuration(total int) string {
	if total >= 60 {
		m := total / 60
		s := total % 60
		return fmt.Sprintf("%dm %ds (%ds)", m, s, total)
	}
	return fmt.Sprintf("%ds", total)
}

// ---------------- restore ----------------
func cmdRestore(args []string) error {
	srv, err := requireServer(args)
	if err != nil {
		return err
	}
	dir := filepath.Join(mcBase(), srv)
	backupFile := ""
	if len(args) > 1 {
		backupFile = args[1]
	}
	if backupFile == "" {
		fmt.Println("Available backups:")
		matches, _ := filepath.Glob("/mnt/HDD/mc/*" + srv + "*.tar.gz")
		// sort by mtime desc, take 5
		type b struct{ name string; t time.Time }
		var bs []b
		for _, m := range matches {
			fi, _ := os.Stat(m)
			bs = append(bs, b{m, fi.ModTime()})
		}
		for i := len(bs) - 1; i >= 0; i-- {
			for j := 0; j < i; j++ {
				if bs[j].t.Before(bs[j+1].t) {
					bs[j], bs[j+1] = bs[j+1], bs[j]
				}
			}
		}
		if len(bs) > 5 {
			bs = bs[:5]
		}
		for _, x := range bs {
			fmt.Println(x.name)
		}
		fmt.Println()
		fmt.Println("Usage: mc restore <server> (backup-file)")
		return nil
	}
	// Resolve the backup file. Accept a full path, a bare filename relative to
	// /mnt/HDD/mc, or a server-prefix glob that uniquely matches one backup.
	backup, err := resolveBackup(srv, backupFile)
	if err != nil {
		return err
	}
	backupFile = backup

	jar, err := findJar(dir)
	if err == nil {
		if serverRunning(jar) {
			fmt.Printf("Stopping %s...\n", srv)
			cmdStop([]string{srv})
		}
	}

	start := time.Now()
	cmd := exec.Command("tar", "-xzf", backupFile, "-C", dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	dur := time.Since(start)
	durStr := formatDuration(int(dur.Seconds()))
	fmt.Printf("Restored %s from %s (%s)\n", srv, backupFile, durStr)
	notify(srv, "Restore finished in "+durStr, "window-attention")
	return nil
}

// resolveBackup finds the backup archive to restore. It accepts, in order of
// preference:
//   - an absolute/relative path that exists
//   - a bare filename inside /mnt/HDD/mc
//   - a glob/prefix that uniquely matches exactly one archive in /mnt/HDD/mc
func resolveBackup(srv, ref string) (string, error) {
	dir := "/mnt/HDD/mc"
	if ref == "" {
		return "", fmt.Errorf("no backup file specified")
	}
	// 1. Existing path (full or relative to cwd)
	if _, err := os.Stat(ref); err == nil {
		return ref, nil
	}
	// 2. Bare filename inside the backup dir
	if _, err := os.Stat(filepath.Join(dir, ref)); err == nil {
		return filepath.Join(dir, ref), nil
	}
	// 3. Glob matching the server's backups
	matches, _ := filepath.Glob(filepath.Join(dir, ref+"*"))
	if len(matches) == 0 {
		matches, _ = filepath.Glob(filepath.Join(dir, "*"+srv+"*.tar.gz"))
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("%d backups match '%s'. Options:\n  %s",
			len(matches), ref, strings.Join(matches, "\n  "))
	}
	return "", fmt.Errorf("Backup file not found: %s", ref)
}

// ---------------- rmworld ----------------
func cmdRmworld(args []string) error {
	srv, err := requireServer(args)
	if err != nil {
		return err
	}
	dir := filepath.Join(mcBase(), srv)
	if _, err := os.Stat(filepath.Join(dir, "world")); err != nil {
		fmt.Printf("No world folder found in %s\n", srv)
		return nil
	}
	fmt.Printf("World folder: %s/world\n", dir)

	fmt.Print("Backup world before deleting? [y/N] ")
	var confirm string
	fmt.Scanln(&confirm)
	if strings.EqualFold(strings.TrimSpace(confirm), "y") {
		dest := "/mnt/HDD/mc"
		os.MkdirAll(dest, 0755)
		ts := time.Now().Format("2006-01-02_1504")
		var bluemapMaps []string
		for _, w := range []string{"world", "world_nether", "world_the_end"} {
			if _, err := os.Stat(filepath.Join(dir, "bluemap/web/maps", w)); err == nil {
				bluemapMaps = append(bluemapMaps, "bluemap/web/maps/"+w)
			}
		}
		argsT := []string{"-czf", filepath.Join(dest, srv+"-world-"+ts+".tar.gz"), "-C", dir, "world", "world_nether", "world_the_end"}
		argsT = append(argsT, bluemapMaps...)
		cmd := exec.Command("tar", argsT...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
		fmt.Printf("Backup created: %s/%s-world-%s.tar.gz\n", dest, srv, ts)
	}
	fmt.Print("WARNING: This will DELETE world data for " + srv + ". Continue? [y/N] ")
	fmt.Scanln(&confirm)
	if strings.EqualFold(strings.TrimSpace(confirm), "y") {
		for _, d := range []string{"world", "world_nether", "world_the_end"} {
			os.RemoveAll(filepath.Join(dir, d))
		}
		for _, w := range []string{"world", "world_nether", "world_the_end"} {
			os.RemoveAll(filepath.Join(dir, "bluemap/web/maps", w))
			os.Remove(filepath.Join(dir, "plugins/BlueMap/maps", w+".conf"))
			os.Remove(filepath.Join(dir, "plugins/Chunky/tasks", w+".properties"))
		}
		fmt.Printf("World deleted for %s\n", srv)
		notify(srv, "World deleted", "message-new-instant")
	} else {
		fmt.Println("Cancelled.")
	}
	return nil
}

// ---------------- status ----------------
func cmdStatus(args []string) error {
	srv, err := requireServer(args)
	if err != nil {
		return err
	}
	dir := filepath.Join(mcBase(), srv)
	session := sessionFor(srv)
	jar, err := findJar(dir)
	if err != nil {
		fmt.Printf("%s: INVALID (no .jar file)\n", srv)
		return nil
	}
	if serverRunning(jar) {
		fmt.Printf("%s: RUNNING\n", srv)
	} else if tmuxHasSession(session) {
		fmt.Printf("%s: STOPPED (stale tmux session)\n", srv)
	} else {
		fmt.Printf("%s: STOPPED\n", srv)
	}
	return nil
}

// ---------------- whitelist ----------------
func cmdWhitelist(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("Usage: mc whitelist <server> (list|add <player>|remove <player>)")
	}
	srv := args[0]
	dir := filepath.Join(mcBase(), srv)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("Server '%s' does not exist", srv)
	}
	session := sessionFor(srv)

	action := ""
	if len(args) > 1 {
		action = args[1]
	}
	if action == "" {
		return fmt.Errorf("Usage: mc whitelist <server> (list|add <player>|remove <player>)")
	}

	switch action {
	case "list":
		if _, err := os.Stat(filepath.Join(dir, "whitelist.json")); err != nil {
			return fmt.Errorf("No whitelist.json found for %s", srv)
		}
		entries, err := parseWhitelist(filepath.Join(dir, "whitelist.json"))
		if err != nil {
			return err
		}
		fmt.Printf("Whitelist for %s:\n", srv)
		if len(entries) == 0 {
			fmt.Println("  (empty)")
		} else {
			for _, e := range entries {
				fmt.Printf("  %16s  (%s)\n", e.Name, e.UUID)
			}
		}
	case "add":
		if len(args) < 3 {
			return fmt.Errorf("Usage: mc whitelist %s add <player>", srv)
		}
		player := args[2]
		if !tmuxHasSession(session) {
			return fmt.Errorf("Server %s is not running.", srv)
		}
		fmt.Printf("Adding %s to %s whitelist...\n", player, srv)
		tmuxSend(session, "whitelist add "+player)
		time.Sleep(1 * time.Second)
		printLastWhitelistLog(dir)
	case "remove":
		if len(args) < 3 {
			return fmt.Errorf("Usage: mc whitelist %s remove <player>", srv)
		}
		player := args[2]
		if !tmuxHasSession(session) {
			return fmt.Errorf("Server %s is not running.", srv)
		}
		fmt.Printf("Removing %s from %s whitelist...\n", player, srv)
		tmuxSend(session, "whitelist remove "+player)
		time.Sleep(1 * time.Second)
		printLastWhitelistLog(dir)
	default:
		return fmt.Errorf("Unknown whitelist action: %s\nUsage: mc whitelist <server> (list|add <player>|remove <player>)", action)
	}
	return nil
}

func printLastWhitelistLog(dir string) {
	b, err := os.ReadFile(filepath.Join(dir, "logs", "latest.log"))
	if err != nil {
		return
	}
	lines := strings.Split(string(b), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "whitelist") {
			// strip the [HH:MM:SS] prefix
			if idx := strings.Index(lines[i], "]: "); idx >= 0 {
				fmt.Println(lines[i][idx+3:])
			}
			return
		}
	}
}

// ---------------- bot ----------------
func cmdBot(args []string) error {
	if len(args) < 1 {
		return botList()
	}
	srv := args[0]
	dir := filepath.Join(mcBase(), srv)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("Server '%s' does not exist", srv)
	}
	sub := ""
	if len(args) > 1 {
		sub = args[1]
	}
	switch sub {
	case "", "list", "ls":
		return botList()
	case "join":
		port := serverPort(dir)
		botJoin(port, args[2:])
		return nil
	case "kick", "stop":
		if len(args) < 3 {
			return fmt.Errorf("Usage: mc bot %s kick <username>", srv)
		}
		if !kickBot(args[2]) {
			return fmt.Errorf("no bot named '%s'", args[2])
		}
		return nil
	default:
		return fmt.Errorf("Usage: mc bot <server> [list | join [username] [-b] | kick <username>]")
	}
}

func botList() error {
	entries, err := os.ReadDir(botsDir())
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		u := e.Name()
		b, _ := os.ReadFile(filepath.Join(botsDir(), u))
		info := strings.TrimSpace(string(b))
		sess := u
		if strings.Contains(info, "bedrock") {
			sess = u + "-b"
		}
		if tmuxHasSession(botSessName(sess)) {
			fmt.Printf("%-16s -> :%s\n", u, info)
		} else {
			os.Remove(filepath.Join(botsDir(), u))
		}
	}
	return nil
}

// botJoin spawns an offline bot in tmux using the node engine. Mirrors
// mc-better-bot's join logic (state file + tmux session, java or bedrock).
func botJoin(port string, extra []string) {
	host := "127.0.0.1"
	user := "laptoo"
	bedrock := false
	for i := 0; i < len(extra); i++ {
		switch extra[i] {
		case "-p", "--port":
			if i+1 < len(extra) {
				port = extra[i+1]
				i++
			}
		case "-h", "--host":
			if i+1 < len(extra) {
				host = extra[i+1]
				i++
			}
		case "-b", "--bedrock":
			bedrock = true
		default:
			user = extra[i]
		}
	}
	if bedrock {
		if port == "" {
			port = "19132"
		}
		if !strings.HasSuffix(botSessName(user), "-b") {
			// session name for bedrock is user-b in the sh version
		}
		if !validUsername(user) {
			fmt.Fprintf(os.Stderr, "mc-better-bot: invalid username '%s' (A-Za-z0-9_, max 16)\n", user)
			os.Exit(1)
		}
	} else {
		if port == "" {
			port = "25565"
		}
		if !validUsername(user) {
			fmt.Fprintf(os.Stderr, "mc-better-bot: invalid username '%s' (A-Za-z0-9_, max 16)\n", user)
			os.Exit(1)
		}
	}
	dieIfRunning(user, bedrock)
	if !portListening(port) {
		fmt.Fprintf(os.Stderr, "mc-better-bot: nothing listening on :%s\n", port)
		os.Exit(1)
	}

	proto := "java"
	if bedrock {
		proto = "bedrock"
	}
	os.MkdirAll(botsDir(), 0755)
	os.WriteFile(filepath.Join(botsDir(), user), []byte(port+" "+proto+"\n"), 0644)

	script := "index.js"
	sess := user
	if bedrock {
		script = "index-bedrock.js"
		sess = user + "-b"
	}
	botDir := filepath.Join(home(), "mc", "bot")
	cmdline := fmt.Sprintf("node %s --host %s --port %s --username %s 2>&1 | tee -a %s/%s.log",
		script, host, port, user, botDir, user)
	exec.Command("tmux", "new-session", "-d", "-s", botSessName(sess), "-c", botDir, cmdline).Run()
	time.Sleep(1 * time.Second)
	if !tmuxHasSession(botSessName(sess)) {
		os.Remove(filepath.Join(botsDir(), user))
		b, _ := os.ReadFile(filepath.Join(botDir, user+".log"))
		lines := strings.Split(string(b), "\n")
		if len(lines) > 5 {
			lines = lines[len(lines)-5:]
		}
		for _, l := range lines {
			fmt.Fprintln(os.Stderr, l)
		}
		fmt.Fprintf(os.Stderr, "mc-better-bot: bot '%s' exited\n", user)
		os.Exit(1)
	}
	fmt.Printf("bot '%s' (%s) joining :%s — console: tmux attach -t %s\n", user, proto, port, botSessName(sess))
}

func validUsername(u string) bool {
	if len(u) == 0 || len(u) > 16 {
		return false
	}
	for _, r := range u {
		if !(r >= 'A' && r <= 'Z') && !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '_' {
			return false
		}
	}
	return true
}

func dieIfRunning(user string, bedrock bool) {
	sess := user
	if bedrock {
		sess = user + "-b"
	}
	if tmuxHasSession(botSessName(sess)) {
		proto := "java"
		if bedrock {
			proto = "bedrock"
		}
		fmt.Fprintf(os.Stderr, "mc-better-bot: bot '%s' (%s) already running\n", user, proto)
		os.Exit(1)
	}
}

func portListening(port string) bool {
	out, _ := exec.Command("ss", "-tlnH").Output()
	if strings.Contains(string(out), ":"+port+" ") {
		return true
	}
	out, _ = exec.Command("ss", "-ulnH").Output()
	return strings.Contains(string(out), ":"+port+" ")
}

func parseWhitelist(path string) ([]struct{ Name, UUID string }, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	type ent struct {
		Name string `json:"name"`
		UUID string `json:"uuid"`
	}
	var list []ent
	// whitelist.json may be an array or a map with "whitelist" array for modern paper
	if err := parseJSON(b, &list); err != nil {
		var m struct {
			Whitelist []ent `json:"whitelist"`
		}
		if err2 := parseJSON(b, &m); err2 != nil {
			return nil, fmt.Errorf("could not parse whitelist: %v", err)
		}
		list = m.Whitelist
	}
	out := make([]struct{ Name, UUID string }, 0, len(list))
	for _, e := range list {
		out = append(out, struct{ Name, UUID string }{e.Name, e.UUID})
	}
	return out, nil
}

func parseJSON(b []byte, v interface{}) error {
	return jsonUnmarshal(b, v)
}

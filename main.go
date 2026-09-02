package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const usageText = `Usage: mc <command> [server]

Commands:
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
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usageText)
		os.Exit(1)
	}
	cmd := os.Args[1]

	if cmd == "-h" || cmd == "--help" || cmd == "help" {
		fmt.Print(usageText)
		os.Exit(0)
	}

	args := os.Args[2:]

	var err error
	switch cmd {
	case "list":
		err = cmdList()
	case "check":
		err = cmdCheck(args)
	case "watch":
		err = cmdWatch()
	case "start":
		err = cmdStart(args)
	case "stop":
		err = cmdStop(args)
	case "restart":
		err = cmdRestart(args)
	case "console":
		err = cmdConsole(args)
	case "logs":
		err = cmdLogs(args)
	case "backup":
		err = cmdBackup(args)
	case "restore":
		err = cmdRestore(args)
	case "rmworld":
		err = cmdRmworld(args)
	case "status":
		err = cmdStatus(args)
	case "whitelist":
		err = cmdWhitelist(args)
	case "bot":
		err = cmdBot(args)
	case "java":
		err = cmdJava(args)
	case "setup":
		err = cmdSetup(args)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		fmt.Print(usageText)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "mc:", err)
		os.Exit(1)
	}
}

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

func mcBase() string {
	return filepath.Join(home(), "mc")
}

// listServers returns directory names under ~/mc that contain at least one .jar.
func listServers() ([]string, error) {
	entries, err := os.ReadDir(mcBase())
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		jars, _ := filepath.Glob(filepath.Join(mcBase(), e.Name(), "*.jar"))
		if len(jars) > 0 {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// findJar returns the first .jar in a server dir (any jar).
func findJar(dir string) (string, error) {
	jars, _ := filepath.Glob(filepath.Join(dir, "*.jar"))
	if len(jars) == 0 {
		return "", fmt.Errorf("no .jar file found in %s", filepath.Base(dir))
	}
	return filepath.Base(jars[0]), nil
}

// readJavaCmd returns the java command override from .java_version, else "java".
func readJavaCmd(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, ".java_version"))
	if err != nil {
		return "java"
	}
	return strings.TrimSpace(string(b))
}

// serverPort reads server-port from server.properties, defaulting to 25565.
func serverPort(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "server.properties"))
	if err != nil {
		return "25565"
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "server-port=") {
			return strings.TrimSpace(strings.TrimPrefix(line, "server-port="))
		}
	}
	return "25565"
}

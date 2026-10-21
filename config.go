package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func jsonUnmarshal(b []byte, v interface{}) error {
	return json.Unmarshal(b, v)
}

// mcArgsLine returns the canonical java command line with paper.jar as the
// jar placeholder (mirrors $mcargs from minecraft.fish {old-mc artifact}). The jar name and
// java provider are substituted per-server at launch.
func mcArgsLine() string {
	return "java -Xms2G -Xmx4G -Djava.io.tmpdir=" + cfg().TmpDir + " -Djava.net.preferIPv4Stack=true -XX:+UseG1GC " +
		"-XX:+ParallelRefProcEnabled -XX:MaxGCPauseMillis=100 " +
		"-XX:+UnlockExperimentalVMOptions -XX:+DisableExplicitGC -XX:+AlwaysPreTouch " +
		"-XX:G1NewSizePercent=30 -XX:G1MaxNewSizePercent=40 -XX:G1HeapRegionSize=8M " +
		"-XX:G1ReservePercent=20 -XX:G1HeapWastePercent=5 -XX:G1MixedGCCountTarget=4 " +
		"-XX:InitiatingHeapOccupancyPercent=15 -XX:G1MixedGCLiveThresholdPercent=90 " +
		"-XX:G1RSetUpdatingPauseTimePercent=5 -XX:SurvivorRatio=32 " +
		"-XX:MaxTenuringThreshold=1 -Dusing.aikars.flags=https://mcflags.emc.gs " +
		"-Daikars.new.flags=true -jar paper.jar --nogui"
}

// defaultJvmFlags is the JVM flags chunk without the leading "java " and
// trailing " -jar paper.jar --nogui" (i.e. just the flags).
func defaultJvmFlags() string {
	line := mcArgsLine()
	line = strings.TrimPrefix(line, "java ")
	if idx := strings.Index(line, " -jar "); idx >= 0 {
		line = line[:idx]
	}
	return strings.TrimSpace(line)
}

// jvmArgsPath returns the per-server args file path (.java_args primary,
// .mc_args / .jvm_args as fallback for reading).
func jvmArgsPath(dir string) string {
	return filepath.Join(dir, ".java_args")
}

func readJvmArgs(dir string) string {
	// try .java_args first, then fallbacks
	for _, name := range []string{".java_args", ".mc_args", ".jvm_args"} {
		p := filepath.Join(dir, name)
		if b, err := os.ReadFile(p); err == nil {
			s := strings.TrimSpace(string(b))
			if s == "" {
				return defaultJvmFlags()
			}
			// if file contains a full command line (has "java " or " -jar "), extract flags
			if strings.HasPrefix(s, "java ") {
				s = strings.TrimPrefix(s, "java ")
			}
			if idx := strings.Index(s, " -jar "); idx >= 0 {
				s = s[:idx]
			}
			// also handle single-line with newlines collapsed
			s = strings.Join(strings.Fields(s), " ")
			if s == "" {
				return defaultJvmFlags()
			}
			return s
		}
	}
	return defaultJvmFlags()
}

func hasCustomJvmArgs(dir string) bool {
	for _, name := range []string{".java_args", ".mc_args", ".jvm_args"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

func writeJvmArgs(dir, flags string) error {
	flags = strings.TrimSpace(strings.Join(strings.Fields(flags), " "))
	if flags == "" {
		return fmt.Errorf("empty flags")
	}
	return os.WriteFile(jvmArgsPath(dir), []byte(flags+"\n"), 0644)
}

func removeJvmArgs(dir string) error {
	for _, name := range []string{".java_args", ".mc_args", ".jvm_args"} {
		_ = os.Remove(filepath.Join(dir, name))
	}
	return nil
}

// parseMem extracts -Xms and -Xmx from a flags string.
func parseMem(flags string) (xms, xmx string) {
	for _, f := range strings.Fields(flags) {
		if strings.HasPrefix(f, "-Xms") {
			xms = f[4:]
		} else if strings.HasPrefix(f, "-Xmx") {
			xmx = f[4:]
		}
	}
	return
}

func updateMem(flags, newXms, newXmx string) string {
	fields := strings.Fields(flags)
	var out []string
	hasXms, hasXmx := false, false
	for _, f := range fields {
		if strings.HasPrefix(f, "-Xms") {
			if newXms != "" {
				out = append(out, "-Xms"+newXms)
				hasXms = true
			}
			continue
		}
		if strings.HasPrefix(f, "-Xmx") {
			if newXmx != "" {
				out = append(out, "-Xmx"+newXmx)
				hasXmx = true
			}
			continue
		}
		out = append(out, f)
	}
	if newXms != "" && !hasXms {
		out = append([]string{"-Xms" + newXms}, out...)
	}
	if newXmx != "" && !hasXmx {
		// insert after Xms if present
		insertAt := 1
		if hasXms {
			insertAt = 1
		} else {
			insertAt = 0
		}
		if insertAt >= len(out) {
			out = append(out, "-Xmx"+newXmx)
		} else {
			tmp := make([]string, 0, len(out)+1)
			tmp = append(tmp, out[:insertAt]...)
			tmp = append(tmp, "-Xmx"+newXmx)
			tmp = append(tmp, out[insertAt:]...)
			out = tmp
		}
	}
	return strings.Join(out, " ")
}

// configPath is the optional runtime config override.
func configPath() string {
	return filepath.Join(home(), ".config/mc/config.json")
}

// Config holds every path/layout assumption that used to be compiled in.
// Defaults reproduce the original laptoo layout; ~/.config/mc/config.json
// overrides any subset of them.
type Config struct {
	ServerRoot    string `json:"server_root"`     // dirs holding one server each
	BackupDir     string `json:"backup_dir"`      // where `mc backup` writes / `restore` reads
	JVMRoot       string `json:"jvm_root"`        // contains <name>/bin/java
	BotDir        string `json:"bot_dir"`         // bot engine + state + logs
	SoundsDir     string `json:"sounds_dir"`      // .ogg files used by notifications
	TmpDir        string `json:"tmp_dir"`         // -Djava.io.tmpdir for default flags
	SessionPrefix string `json:"session_prefix"`  // tmux session = <prefix><server>
	NtfyURL       string `json:"ntfy_url"`        // local ntfy base URL
	NtfyTokenFile string `json:"ntfy_token_file"` // file containing NTFY_TOKEN=...
}

func defaultConfig() Config {
	return Config{
		ServerRoot:    filepath.Join(home(), "mc"),
		BackupDir:     "/mnt/HDD/mc",
		JVMRoot:       "/usr/lib/jvm",
		BotDir:        filepath.Join(home(), "mc", "bot"),
		SoundsDir:     filepath.Join(home(), "mc", "library", "sounds"),
		TmpDir:        "/var/tmp",
		SessionPrefix: "mc-",
		NtfyURL:       "http://127.0.0.1:8989",
		NtfyTokenFile: "/etc/sysmon/tokens/ntfy.sh",
	}
}

var (
	configOnce sync.Once
	configVal  Config
)

// cfg returns the process-wide config, loading it once.
func cfg() Config {
	configOnce.Do(func() { configVal = loadConfig() })
	return configVal
}

func loadConfig() Config {
	c := defaultConfig()
	b, err := os.ReadFile(configPath())
	if err != nil {
		return c
	}
	if err := json.Unmarshal(b, &c); err != nil {
		fmt.Fprintf(os.Stderr, "mc: warning: ignoring %s: %v\n", configPath(), err)
		return defaultConfig()
	}
	c.applyDefaults()
	c.ServerRoot = expandPath(c.ServerRoot)
	c.BackupDir = expandPath(c.BackupDir)
	c.JVMRoot = expandPath(c.JVMRoot)
	c.BotDir = expandPath(c.BotDir)
	c.SoundsDir = expandPath(c.SoundsDir)
	c.TmpDir = expandPath(c.TmpDir)
	c.NtfyTokenFile = expandPath(c.NtfyTokenFile)
	return c
}

// applyDefaults fills anything left empty (e.g. `"backup_dir": ""`).
func (c *Config) applyDefaults() {
	d := defaultConfig()
	if c.ServerRoot == "" {
		c.ServerRoot = d.ServerRoot
	}
	if c.BackupDir == "" {
		c.BackupDir = d.BackupDir
	}
	if c.JVMRoot == "" {
		c.JVMRoot = d.JVMRoot
	}
	if c.BotDir == "" {
		c.BotDir = d.BotDir
	}
	if c.SoundsDir == "" {
		c.SoundsDir = d.SoundsDir
	}
	if c.TmpDir == "" {
		c.TmpDir = d.TmpDir
	}
	if c.SessionPrefix == "" {
		c.SessionPrefix = d.SessionPrefix
	}
	if c.NtfyURL == "" {
		c.NtfyURL = d.NtfyURL
	}
	if c.NtfyTokenFile == "" {
		c.NtfyTokenFile = d.NtfyTokenFile
	}
}

// expandPath expands a leading ~ and any $VARS.
func expandPath(p string) string {
	if p == "~" {
		p = home()
	} else if strings.HasPrefix(p, "~/") {
		p = filepath.Join(home(), p[2:])
	}
	return os.ExpandEnv(p)
}

// tildePath turns a path under $HOME back into ~/... so a copied config keeps
// working for a different user.
func tildePath(p string) string {
	h := home()
	if p == h {
		return "~"
	}
	if strings.HasPrefix(p, h+string(filepath.Separator)) {
		return "~/" + filepath.ToSlash(strings.TrimPrefix(p, h+string(filepath.Separator)))
	}
	return p
}

func saveConfig(c Config) error {
	p := configPath()
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	c.ServerRoot = tildePath(c.ServerRoot)
	c.BackupDir = tildePath(c.BackupDir)
	c.JVMRoot = tildePath(c.JVMRoot)
	c.BotDir = tildePath(c.BotDir)
	c.SoundsDir = tildePath(c.SoundsDir)
	c.TmpDir = tildePath(c.TmpDir)
	c.NtfyTokenFile = tildePath(c.NtfyTokenFile)
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0644)
}

func setConfigField(c *Config, key, val string) error {
	key = strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	switch key {
	case "server_root":
		c.ServerRoot = val
	case "backup_dir":
		c.BackupDir = val
	case "jvm_root":
		c.JVMRoot = val
	case "bot_dir":
		c.BotDir = val
	case "sounds_dir":
		c.SoundsDir = val
	case "tmp_dir":
		c.TmpDir = val
	case "session_prefix":
		c.SessionPrefix = val
	case "ntfy_url":
		c.NtfyURL = val
	case "ntfy_token_file":
		c.NtfyTokenFile = val
	default:
		return fmt.Errorf("unknown config key %q\nvalid keys: server_root backup_dir jvm_root bot_dir sounds_dir tmp_dir session_prefix ntfy_url ntfy_token_file", key)
	}
	return nil
}

func cmdConfig(args []string) error {
	p := configPath()
	if len(args) == 0 || args[0] == "show" {
		printConfig(p)
		return nil
	}
	switch args[0] {
	case "path":
		fmt.Println(p)
	case "init":
		force := len(args) > 1 && (args[1] == "-f" || args[1] == "--force")
		if _, err := os.Stat(p); err == nil && !force {
			return fmt.Errorf("%s already exists (use 'mc config init --force' to overwrite)", p)
		}
		if err := saveConfig(defaultConfig()); err != nil {
			return err
		}
		fmt.Printf("Wrote default config to %s\n", p)
	case "set":
		if len(args) < 3 {
			return fmt.Errorf("Usage: mc config set <key> <value>")
		}
		key, val := args[1], strings.Join(args[2:], " ")
		c := cfg()
		if err := setConfigField(&c, key, val); err != nil {
			return err
		}
		if err := saveConfig(c); err != nil {
			return err
		}
		fmt.Printf("%s = %s\n", strings.ToLower(strings.ReplaceAll(key, "-", "_")), val)
	case "help", "-h", "--help":
		printConfigHelp()
	default:
		return fmt.Errorf("Usage: mc config [show|path|init [--force]|set <key> <value>]")
	}
	return nil
}

func printConfigHelp() {
	fmt.Print(`Usage: mc config [show|path|init [--force]|set <key> <value>]

  mc config                 Show effective values + config file path
  mc config path            Print the config file path
  mc config init [--force]  Write a default config file
  mc config set <key> <val> Update one key (writes the whole file)

Keys:
  server_root      servers live here, one dir per server      (~/mc)
  backup_dir       backup archives read/written here          (/mnt/HDD/mc)
  jvm_root         scanned as <jvm_root>/*/bin/java            (/usr/lib/jvm)
  bot_dir          bot engine, .bots state and logs            (~/mc/bot)
  sounds_dir       .ogg files for desktop notifications        (~/mc/library/sounds)
  tmp_dir          -Djava.io.tmpdir in the default flags       (/var/tmp)
  session_prefix   tmux session name = <prefix><server>        (mc-)
  ntfy_url         local ntfy base URL                         (http://127.0.0.1:8989)
  ntfy_token_file  file containing NTFY_TOKEN=...              (/etc/sysmon/tokens/ntfy.sh)

Values may use ~ and $VARS. Missing or empty keys fall back to the defaults.
`)
}

func printConfig(p string) {
	c := cfg()
	set := map[string]bool{}
	status := "using defaults"
	if b, err := os.ReadFile(p); err == nil {
		var raw map[string]json.RawMessage
		if json.Unmarshal(b, &raw) == nil {
			status = "loaded"
			for k := range raw {
				set[strings.ToLower(k)] = true
			}
		} else {
			status = "unreadable — using defaults"
		}
	}
	fmt.Printf("Config file: %s (%s)\n\n", p, status)

	rows := []struct {
		key string
		val string
	}{
		{"server_root", c.ServerRoot},
		{"backup_dir", c.BackupDir},
		{"jvm_root", c.JVMRoot},
		{"bot_dir", c.BotDir},
		{"sounds_dir", c.SoundsDir},
		{"tmp_dir", c.TmpDir},
		{"session_prefix", c.SessionPrefix},
		{"ntfy_url", c.NtfyURL},
		{"ntfy_token_file", c.NtfyTokenFile},
	}
	for _, r := range rows {
		src := "default"
		if set[r.key] {
			src = "file"
		}
		fmt.Printf("  %-16s %-40s (%s)\n", r.key, r.val, src)
	}
}

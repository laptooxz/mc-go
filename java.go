package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// jvm holds a discovered Java installation.
type jvm struct {
	Path    string // e.g. /usr/lib/jvm/java-21-openjdk/bin/java
	Label   string // e.g. java-21-openjdk
	Version string // e.g. 21.0.12
	Raw     string // first line of java -version (e.g. openjdk version "21.0.12" 2026-07-21)
}

// listJVMs scans /usr/lib/jvm for Java installs.
func listJVMs() []jvm {
	matches, _ := filepath.Glob("/usr/lib/jvm/*/bin/java")
	seen := map[string]bool{}
	var out []jvm
	for _, p := range matches {
		// resolve label: .../jvm/<name>/bin/java -> <name>
		label := filepath.Base(filepath.Dir(filepath.Dir(p)))
		if strings.Contains(label, "default") {
			continue
		}
		real, _ := filepath.EvalSymlinks(p)
		if seen[real] {
			continue
		}
		seen[real] = true
		ver, raw := javaVersion(p)
		out = append(out, jvm{Path: p, Label: label, Version: ver, Raw: raw})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Label < out[j].Label
	})
	return out
}

func javaVersion(javaPath string) (string, string) {
	out, err := exec.Command(javaPath, "-version").CombinedOutput()
	if err != nil && len(out) == 0 {
		return "?", ""
	}
	first := strings.TrimSpace(strings.Split(string(out), "\n")[0])
	// first line like: openjdk version "21.0.12" 2026-07-21
	ver := "?"
	if s := strings.Index(first, "\""); s >= 0 {
		if e := strings.Index(first[s+1:], "\""); e >= 0 {
			ver = first[s+1 : s+1+e]
		}
	}
	return ver, first
}

// javaStatus describes current .java_version state for a server.
type javaStatus struct {
	Server  string
	Dir     string
	Jar     string
	Path    string // current .java_version content (or "" if missing)
	Exists  bool   // .java_version file exists
	Valid   bool   // file points to an executable java
	Label   string // label of the resolved jvm if found in list
	Version string // java -version if valid
}

func checkJavaForServer(srv string) javaStatus {
	dir := filepath.Join(mcBase(), srv)
	jar, _ := findJar(dir)
	js := javaStatus{Server: srv, Dir: dir, Jar: jar}
	b, err := os.ReadFile(filepath.Join(dir, ".java_version"))
	if err != nil {
		js.Path = ""
		js.Exists = false
		return js
	}
	js.Exists = true
	js.Path = strings.TrimSpace(string(b))
	if js.Path == "" {
		return js
	}
	// allow bare "java" (fallback to PATH) — treat as valid but unknown version
	if js.Path == "java" {
		js.Valid = true
		js.Label = "java (PATH)"
		if v, _ := javaVersion("java"); v != "?" {
			js.Version = v
		}
		return js
	}
	if fi, err := os.Stat(js.Path); err == nil && fi.Mode()&0111 != 0 {
		js.Valid = true
		if v, _ := javaVersion(js.Path); v != "?" {
			js.Version = v
		}
		// try to map to a known label
		real, _ := filepath.EvalSymlinks(js.Path)
		for _, j := range listJVMs() {
			r, _ := filepath.EvalSymlinks(j.Path)
			if r == real || j.Path == js.Path {
				js.Label = j.Label
				break
			}
		}
		if js.Label == "" {
			js.Label = filepath.Base(filepath.Dir(filepath.Dir(js.Path)))
		}
	}
	return js
}

func suggestJava(jar string) string {
	j := strings.ToLower(jar)
	switch {
	case strings.Contains(j, "1.7") || strings.Contains(j, "1.8") || strings.Contains(j, "1.12"):
		return "java-8-openjdk  (8)  — MC 1.7–1.12"
	case strings.Contains(j, "1.16"):
		return "java-8-openjdk or java-17-openjdk  — MC 1.16"
	case strings.Contains(j, "1.17"):
		return "java-17-openjdk  (17)  — MC 1.17"
	case strings.Contains(j, "1.18"), strings.Contains(j, "1.19"), strings.Contains(j, "1.20") && !strings.Contains(j, "26."):
		return "java-17-openjdk  (17)  — MC 1.18–1.20"
	case strings.Contains(j, "26."):
		return "java-25-openjdk  (25)  — MC 26.x  (also 21)"
	default:
		if strings.Contains(j, "paper") || j == "paper.jar" {
			return "java-21-openjdk or java-25-openjdk  — modern Paper"
		}
	}
	return ""
}

// ---------------- cmdJava ----------------

func cmdJava(args []string) error {
	jvms := listJVMs()
	if len(jvms) == 0 {
		return fmt.Errorf("no JVMs found in /usr/lib/jvm/*/bin/java")
	}

	// sub-modes: list, check, <server> [java-ref]
	if len(args) > 0 {
		switch args[0] {
		case "list", "ls":
			fmt.Println("Installed JVMs:")
			for _, j := range jvms {
				fmt.Printf("  %-22s %-42s  %s\n", j.Label, j.Path, j.Version)
				if j.Raw != "" {
					fmt.Printf("    %s\n", j.Raw)
				}
			}
			return nil
		case "check":
			return javaCheckAll(jvms)
		case "help", "-h", "--help":
			printJavaHelp()
			return nil
		}
	}

	// mc java <server> [<java-ref>]
	if len(args) >= 1 {
		srv := args[0]
		// validate server exists (allow any dir under ~/mc, not just those with jars — .java_version is per-dir)
		if _, err := os.Stat(filepath.Join(mcBase(), srv)); err != nil {
			return fmt.Errorf("Server '%s' does not exist", srv)
		}
		if len(args) >= 2 {
			// non-interactive set: mc java <server> <path|label|version-number>
			ref := strings.Join(args[1:], " ")
			target, err := resolveJVMRef(jvms, ref)
			if err != nil {
				return err
			}
			return writeJavaVersion(srv, target.Path)
		}
		// interactive: pick JVM for this server
		return javaPickForServer(srv, jvms)
	}

	// no args: check-all + interactive server picker
	if err := javaCheckAll(jvms); err != nil {
		return err
	}
	fmt.Println()
	srv, err := pickServerFzf()
	if err != nil {
		return err
	}
	if srv == "" {
		fmt.Println("Cancelled.")
		return nil
	}
	return javaPickForServer(srv, jvms)
}

func printJavaHelp() {
	fmt.Print(`Usage: mc java [server] [jvm]

  mc java                Check all servers + pick server & JVM via fzf
  mc java check          Check all servers' .java_version (no picker)
  mc java list           List installed JVMs
  mc java <server>       Pick JVM for <server> via fzf
  mc java <server> <jvm> Set JVM non-interactively
                         <jvm> can be:  8 | 17 | 21 | 25
                                         java-21-openjdk
                                         /usr/lib/jvm/java-21-openjdk/bin/java

Examples:
  mc java
  mc java survival
  mc java survival 21
  mc java survival java-17-openjdk
  mc java check
`)
}

func javaCheckAll(jvms []jvm) error {
	servers, err := listServers()
	if err != nil {
		return err
	}
	// also include dirs without jars but with .java_version or with server.properties
	// listServers only returns dirs with jars; extend to all mc subdirs that look like servers
	allDirs, _ := os.ReadDir(mcBase())
	seen := map[string]bool{}
	for _, s := range servers {
		seen[s] = true
	}
	for _, e := range allDirs {
		if !e.IsDir() || seen[e.Name()] {
			continue
		}
		n := e.Name()
		if n == "bot" || n == "library" {
			continue
		}
		// include if it has server.properties or .java_version
		if _, err := os.Stat(filepath.Join(mcBase(), n, "server.properties")); err == nil {
			servers = append(servers, n)
		} else if _, err := os.Stat(filepath.Join(mcBase(), n, ".java_version")); err == nil {
			servers = append(servers, n)
		}
	}
	sort.Strings(servers)

	fmt.Printf("%-18s %-10s %-30s %s\n", "SERVER", "JAVA", "VERSION FILE", "JAR / NOTE")
	fmt.Println(strings.Repeat("-", 90))
	for _, srv := range servers {
		st := checkJavaForServer(srv)
		status := ""
		verFile := ""
		note := st.Jar
		if !st.Exists {
			status = "MISSING"
			verFile = "(none → fallback 'java')"
			if s := suggestJava(st.Jar); s != "" {
				note = st.Jar + "  → suggest: " + s
			}
		} else if !st.Valid {
			status = "INVALID"
			verFile = st.Path
			note = "path not executable"
		} else {
			if st.Label != "" {
				status = st.Label
			} else {
				status = "custom"
			}
			verFile = st.Path
			if st.Version != "" {
				verFile = verFile + "  (" + st.Version + ")"
			}
		}
		icon := "✓"
		if !st.Exists || !st.Valid {
			icon = "⚠"
		}
		fmt.Printf("%-18s %-10s %-30s %s  %s\n", srv, status, truncate(verFile, 30), note, icon)
	}
	fmt.Println(strings.Repeat("-", 90))
	fmt.Printf("JVMs: ")
	for i, j := range jvms {
		if i > 0 {
			fmt.Print("  ")
		}
		fmt.Printf("%s(%s)", j.Label, j.Version)
	}
	fmt.Println()
	_ = jvms
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func writeJavaVersion(srv, javaPath string) error {
	dir := filepath.Join(mcBase(), srv)
	p := filepath.Join(dir, ".java_version")
	if err := os.WriteFile(p, []byte(strings.TrimSpace(javaPath)+"\n"), 0644); err != nil {
		return err
	}
	st := checkJavaForServer(srv)
	fmt.Printf("Set %s/.java_version → %s", srv, javaPath)
	if st.Valid && st.Version != "" {
		fmt.Printf("  (%s)", st.Version)
	}
	fmt.Println()
	if s := suggestJava(st.Jar); s != "" && !strings.Contains(javaPath, "25") && !strings.Contains(javaPath, "21") && strings.Contains(st.Jar, "26.") {
		fmt.Printf("  note: %s suggests %s\n", st.Jar, s)
	}
	return nil
}

func resolveJVMRef(jvms []jvm, ref string) (jvm, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return jvm{}, fmt.Errorf("empty JVM reference")
	}
	// direct path
	if strings.Contains(ref, "/") {
		if _, err := os.Stat(ref); err == nil {
			ver, _ := javaVersion(ref)
			return jvm{Path: ref, Label: filepath.Base(filepath.Dir(filepath.Dir(ref))), Version: ver}, nil
		}
		return jvm{}, fmt.Errorf("path not found: %s", ref)
	}
	// numeric shorthand: 8, 17, 21, 25
	norm := strings.ToLower(ref)
	norm = strings.TrimPrefix(norm, "java-")
	norm = strings.TrimPrefix(norm, "java")
	norm = strings.Trim(norm, "- ")
	for _, j := range jvms {
		// label contains the major version
		if strings.Contains(j.Label, norm) {
			return j, nil
		}
		if j.Version != "" && strings.HasPrefix(j.Version, norm+".") {
			return j, nil
		}
		if j.Version == norm {
			return j, nil
		}
	}
	// try exact label match
	for _, j := range jvms {
		if strings.EqualFold(j.Label, ref) || strings.EqualFold(j.Label, "java-"+ref) {
			return j, nil
		}
	}
	return jvm{}, fmt.Errorf("no JVM matches '%s'. Try: mc java list", ref)
}

// pickServerFzf shows a fzf menu of servers with their java status and returns the chosen server name.
func pickServerFzf() (string, error) {
	servers, _ := listServers()
	if len(servers) == 0 {
		return "", fmt.Errorf("no servers found in ~/mc")
	}
	// enrich lines with java status
	var lines []string
	width := 0
	for _, s := range servers {
		if len(s) > width {
			width = len(s)
		}
	}
	for _, srv := range servers {
		st := checkJavaForServer(srv)
		badge := ""
		if !st.Exists {
			badge = "MISSING"
		} else if !st.Valid {
			badge = "INVALID"
		} else if st.Label != "" {
			badge = st.Label
		} else {
			badge = "custom"
		}
		ver := ""
		if st.Valid && st.Version != "" {
			ver = st.Version
		}
		line := fmt.Sprintf("%-*s  %-18s  %-10s  %s", width, srv, "["+badge+"]", ver, st.Jar)
		lines = append(lines, line)
	}
	sort.Strings(lines)

	choice, err := runFzf(lines, "server> ", "Pick a server to configure .java_version  (ESC to cancel)")
	if err != nil {
		return "", err
	}
	if choice == "" {
		return "", nil
	}
	// first field is server name
	fields := strings.Fields(choice)
	if len(fields) == 0 {
		return "", nil
	}
	return fields[0], nil
}

func javaPickForServer(srv string, jvms []jvm) error {
	st := checkJavaForServer(srv)
	fmt.Printf("Server: %s  jar: %s\n", srv, st.Jar)
	if st.Exists {
		fmt.Printf("Current: %s", st.Path)
		if st.Valid && st.Version != "" {
			fmt.Printf("  (%s %s)", st.Label, st.Version)
		} else if !st.Valid {
			fmt.Printf("  (INVALID)")
		}
		fmt.Println()
	} else {
		fmt.Println("Current: (none) → fallback 'java' →", readJavaCmd(filepath.Join(mcBase(), srv)))
	}
	if s := suggestJava(st.Jar); s != "" {
		fmt.Printf("Suggest: %s\n", s)
	}
	fmt.Println()

	// Build fzf input: one line per JVM, with current marker
	var lines []string
	for _, j := range jvms {
		marker := "  "
		if st.Valid && st.Path == j.Path {
			marker = "● "
		} else if st.Label == j.Label && st.Valid {
			marker = "● "
		}
		// pretty line: keep path as last field for easy extraction but show nicely
		line := fmt.Sprintf("%s%-22s  %-42s  %s  ::%s", marker, j.Label, j.Path, j.Version, j.Path)
		lines = append(lines, line)
	}

	header := fmt.Sprintf("Pick JVM for %-12s  (current: %s)  — ESC to cancel", srv, st.Path)
	if !st.Exists {
		header = fmt.Sprintf("Pick JVM for %-12s  (none set)  — ESC to cancel", srv)
	}

	choice, err := runFzf(lines, "java> ", header)
	if err != nil {
		return err
	}
	if choice == "" {
		fmt.Println("Cancelled.")
		return nil
	}
	// path is after ::
	path := choice
	if idx := strings.LastIndex(choice, "::"); idx >= 0 {
		path = strings.TrimSpace(choice[idx+2:])
	} else {
		// fallback: last field
		f := strings.Fields(choice)
		path = f[len(f)-1]
	}
	return writeJavaVersion(srv, path)
}

// runFzf pipes lines into fzf and returns the selected line (trimmed).
// If fzf is not available or not a TTY, falls back to a numbered prompt.
func runFzf(lines []string, prompt, header string) (string, error) {
	if _, err := exec.LookPath("fzf"); err != nil {
		return runFallbackPicker(lines, prompt)
	}
	if !isTTY(os.Stdin) || !isTTY(os.Stdout) {
		// non-interactive: fallback
		return runFallbackPicker(lines, prompt)
	}
	cmd := exec.Command("fzf",
		"--prompt="+prompt,
		"--header="+header,
		"--height=45%",
		"--layout=reverse",
		"--border=rounded",
		"--info=inline",
		"--ansi",
		"--no-multi",
	)
	cmd.Stderr = os.Stderr
	in, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		return runFallbackPicker(lines, prompt)
	}
	go func() {
		w := bufio.NewWriter(in)
		for _, l := range lines {
			fmt.Fprintln(w, l)
		}
		w.Flush()
		in.Close()
	}()
	buf := new(strings.Builder)
	sc := bufio.NewScanner(out)
	var selected string
	for sc.Scan() {
		selected += sc.Text()
		buf.WriteString(sc.Text() + "\n")
	}
	err := cmd.Wait()
	if err != nil {
		// fzf returns 130 on ESC/Ctrl-C → treat as cancel, not error
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 130 {
			return "", nil
		}
		return "", nil
	}
	return strings.TrimSpace(selected), nil
}

func runFallbackPicker(lines []string, prompt string) (string, error) {
	fmt.Println(prompt)
	for i, l := range lines {
		fmt.Printf("  %2d) %s\n", i+1, strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(l, "● "), "  ")))
	}
	fmt.Printf("Pick number (1-%d, 0 to cancel): ", len(lines))
	var inp string
	fmt.Scanln(&inp)
	inp = strings.TrimSpace(inp)
	if inp == "" || inp == "0" || strings.EqualFold(inp, "q") {
		return "", nil
	}
	var idx int
	if _, err := fmt.Sscan(inp, &idx); err != nil {
		return "", nil
	}
	if idx < 1 || idx > len(lines) {
		return "", nil
	}
	line := lines[idx-1]
	if pos := strings.LastIndex(line, "::"); pos >= 0 {
		return strings.TrimSpace(line[pos+2:]), nil
	}
	f := strings.Fields(line)
	return f[len(f)-1], nil
}

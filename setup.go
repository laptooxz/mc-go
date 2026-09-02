package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func cmdSetup(args []string) error {
	// mc setup [server] — unified java + args configurator
	if len(args) > 0 {
		switch args[0] {
		case "help", "-h", "--help":
			printSetupHelp()
			return nil
		case "check":
			jvms := listJVMs()
			fmt.Println("=== Java check ===")
			_ = javaCheckAll(jvms)
			fmt.Println("\n=== Args check ===")
			return setupCheckAll()
		case "list":
			jvms := listJVMs()
			for _, j := range jvms {
				fmt.Printf("  %-22s %-42s  %s\n", j.Label, j.Path, j.Version)
			}
			return nil
		}
		// if first arg is a server name, enter setup loop for it
		srv := args[0]
		if _, err := os.Stat(filepath.Join(mcBase(), srv)); err == nil {
			// allow extra non-interactive shorthands:
			//   mc setup <srv> java [ref]
			//   mc setup <srv> ram <Xmx> [Xms]
			//   mc setup <srv> flags <flags...>
			if len(args) >= 2 {
				switch args[1] {
				case "java":
					jvms := listJVMs()
					if len(args) >= 3 {
						ref := strings.Join(args[2:], " ")
						tgt, err := resolveJVMRef(jvms, ref)
						if err != nil {
							return err
						}
						return writeJavaVersion(srv, tgt.Path)
					}
					return javaPickForServer(srv, jvms)
				case "ram", "mem", "memory":
					if len(args) >= 3 {
						xmx := args[2]
						xms := ""
						if len(args) >= 4 {
							xms = args[3]
						}
						return setupSetRam(srv, xms, xmx)
					}
					return setupPickRam(srv)
				case "flags", "args":
					dir := filepath.Join(mcBase(), srv)
					if len(args) >= 3 {
						flags := strings.Join(args[2:], " ")
						return writeJvmArgs(dir, flags)
					}
					return setupEditFlags(srv)
				}
			}
			return setupLoop(srv)
		}
		return fmt.Errorf("Server '%s' does not exist", args[0])
	}
	// no args: pick server via fzf
	srv, err := pickServerFzf()
	if err != nil {
		return err
	}
	if srv == "" {
		fmt.Println("Cancelled.")
		return nil
	}
	return setupLoop(srv)
}

func printSetupHelp() {
	fmt.Print(`Usage: mc setup [server] [action]

  mc setup                Pick server via fzf, then configure Java + RAM + flags
  mc setup <server>       Configure <server> (interactive fzf menu)
  mc setup <server> java [jvm]     Set/list Java for server (same as 'mc java')
  mc setup <server> ram [Xmx] [Xms] Set RAM (e.g. 'mc setup survival ram 4G' or '4G 2G')
  mc setup <server> flags [flags]  Set raw JVM flags for server
  mc setup check          Check Java + Args for all servers
  mc setup list           List installed JVMs

Setup menu (interactive):
  Java  — pick JVM via fzf  (writes ~/mc/<srv>/.java_version)
  RAM   — pick Xms/Xmx via fzf (writes ~/mc/<srv>/.java_args)
  Flags — edit raw JVM flags (Aikar defaults + your overrides)
  View  — show full launch command
  Reset — delete .java_args → back to defaults

Files:
  ~/mc/<server>/.java_version  — path to java binary (e.g. /usr/lib/jvm/java-21-openjdk/bin/java)
  ~/mc/<server>/.java_args     — JVM flags (e.g. -Xms2G -Xmx4G -XX:+UseG1GC ...)
                                Falls back to built-in Aikar flags if missing.
                                Also checked: .mc_args, .jvm_args

Examples:
  mc setup
  mc setup survival
  mc setup survival java 21
  mc setup survival ram 6G
  mc setup survival ram 8G 4G
  mc setup check
`)
}

func setupLoop(srv string) error {
	dir := filepath.Join(mcBase(), srv)
	for {
		st := checkJavaForServer(srv)
		flags := readJvmArgs(dir)
		xms, xmx := parseMem(flags)
		custom := ""
		if hasCustomJvmArgs(dir) {
			custom = "custom"
		} else {
			custom = "default"
		}
		// build preview header
		jar, _ := findJar(dir)
		header := fmt.Sprintf("%s  jar:%s  java:%s  ram:%s/%s  flags:%s  — pick an action (ESC to exit)", srv, jar, st.Label, xms, xmx, custom)
		if !st.Exists {
			header = fmt.Sprintf("%s  [java MISSING]  jar:%s  ram:%s/%s — pick an action", srv, jar, xms, xmx)
		} else if !st.Valid {
			header = fmt.Sprintf("%s  [java INVALID: %s]  jar:%s — pick an action", srv, st.Path, jar)
		}

		javaBadge := st.Path
		if st.Label != "" {
			javaBadge = st.Label + "  " + st.Version
		}
		if !st.Exists {
			javaBadge = "MISSING → " + readJavaCmd(dir) + " (fallback 'java')"
		} else if !st.Valid {
			javaBadge = "INVALID: " + st.Path
		}

		lines := []string{
			fmt.Sprintf("Java   %-45s  ::java", truncate(javaBadge, 45)),
			fmt.Sprintf("RAM    Xms=%-8s Xmx=%-8s  (%s)  ::ram", xms, xmx, custom),
			fmt.Sprintf("Flags  %s  ::flags", truncate(flags, 55)),
			fmt.Sprintf("View   show full launch command              ::view"),
			fmt.Sprintf("Reset  delete .java_args → restore defaults  ::reset"),
			fmt.Sprintf("Exit   done                                  ::exit"),
		}

		choice, err := runFzf(lines, "setup> ", header)
		if err != nil {
			return err
		}
		if choice == "" {
			fmt.Println("Exit.")
			return nil
		}
		tag := ""
		if idx := strings.LastIndex(choice, "::"); idx >= 0 {
			tag = strings.TrimSpace(choice[idx+2:])
		}
		switch tag {
		case "java":
			jvms := listJVMs()
			if err := javaPickForServer(srv, jvms); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		case "ram":
			if err := setupPickRam(srv); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		case "flags":
			if err := setupEditFlags(srv); err != nil {
				fmt.Fprintln(os.Stderr, err)
			}
		case "view":
			fmt.Println()
			fmt.Printf("  Server : %s\n", srv)
			fmt.Printf("  Java   : %s\n", readJavaCmd(dir))
			fmt.Printf("  Jar    : %s\n", jar)
			fmt.Printf("  Flags  : %s\n", flags)
			fmt.Printf("  Custom : %v (%s)\n", hasCustomJvmArgs(dir), jvmArgsPath(dir))
			fmt.Println()
			fmt.Println("  Full command:")
			fmt.Printf("    %s\n", runCmd(srv))
			fmt.Println()
			fmt.Print("Press Enter to continue...")
			bufio.NewReader(os.Stdin).ReadString('\n')
		case "reset":
			if hasCustomJvmArgs(dir) {
				fmt.Printf("Delete %s ? [y/N] ", jvmArgsPath(dir))
				var ans string
				fmt.Scanln(&ans)
				if strings.EqualFold(strings.TrimSpace(ans), "y") {
					_ = removeJvmArgs(dir)
					fmt.Println("Reset to defaults.")
				}
			} else {
				fmt.Println("Already on defaults (no .java_args).")
				fmt.Print("Press Enter...")
				bufio.NewReader(os.Stdin).ReadString('\n')
			}
		case "exit":
			fmt.Println("Done.")
			return nil
		default:
			fmt.Println("Unknown:", choice)
		}
	}
}

func setupCheckAll() error {
	servers, err := listServers()
	if err != nil {
		return err
	}
	// include dirs with .java_args or server.properties even without jars
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
		if _, err := os.Stat(filepath.Join(mcBase(), n, "server.properties")); err == nil {
			servers = append(servers, n)
		} else if _, err := os.Stat(filepath.Join(mcBase(), n, ".java_version")); err == nil {
			servers = append(servers, n)
		} else if hasCustomJvmArgs(filepath.Join(mcBase(), n)) {
			servers = append(servers, n)
		}
	}
	// sort handled in javaCheckAll, do here too
	// use simple sort
	for i := 0; i < len(servers); i++ {
		for j := i + 1; j < len(servers); j++ {
			if servers[j] < servers[i] {
				servers[i], servers[j] = servers[j], servers[i]
			}
		}
	}
	fmt.Printf("%-16s %-8s %-8s  %-10s  %s\n", "SERVER", "XMS", "XMX", "FLAGS", "JAVA")
	fmt.Println(strings.Repeat("-", 80))
	for _, srv := range servers {
		dir := filepath.Join(mcBase(), srv)
		st := checkJavaForServer(srv)
		flags := readJvmArgs(dir)
		xms, xmx := parseMem(flags)
		custom := "default"
		if hasCustomJvmArgs(dir) {
			custom = "custom"
		}
		javaLbl := st.Label
		if !st.Exists {
			javaLbl = "MISSING"
		} else if !st.Valid {
			javaLbl = "INVALID"
		}
		if javaLbl == "" {
			javaLbl = "custom"
		}
		fmt.Printf("%-16s %-8s %-8s  %-10s  %s\n", srv, xms, xmx, custom, javaLbl)
	}
	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("Defaults: %s\n", defaultJvmFlags())
	return nil
}

func setupSetRam(srv, xms, xmx string) error {
	dir := filepath.Join(mcBase(), srv)
	flags := readJvmArgs(dir)
	// if only one arg given and caller used ram <Xmx>, treat as Xmx, keep Xms as is or set to half?
	if xms == "" && xmx != "" {
		// keep existing Xms
		curXms, _ := parseMem(flags)
		xms = curXms
		if xms == "" {
			xms = "2G"
		}
	}
	// if both provided via "mc setup <srv> ram 8G 4G" we mapped Xmx first; swap if needed?
	// our mapping above: args[2]=Xmx, args[3]=Xms — but check common usage: user says "ram 4G" meaning Xmx 4G.
	// if two args, treat first as Xmx, second as Xms for flexibility.
	newFlags := updateMem(flags, xms, xmx)
	if err := writeJvmArgs(dir, newFlags); err != nil {
		return err
	}
	nxms, nxmx := parseMem(newFlags)
	fmt.Printf("Set %s RAM → Xms%s Xmx%s\n", srv, nxms, nxmx)
	return nil
}

func setupPickRam(srv string) error {
	dir := filepath.Join(mcBase(), srv)
	flags := readJvmArgs(dir)
	curXms, curXmx := parseMem(flags)
	fmt.Printf("Current RAM for %s: Xms%s Xmx%s\n", srv, curXms, curXmx)

	// Pick Xmx via fzf
	memOptions := []string{
		"512M", "1G", "2G", "3G", "4G", "6G", "8G", "10G", "12G", "16G", "custom...",
	}
	// mark current
	var xmxLines []string
	for _, m := range memOptions {
		marker := "  "
		if m == curXmx {
			marker = "● "
		}
		xmxLines = append(xmxLines, marker+m+" ::"+m)
	}
	choice, err := runFzf(xmxLines, "Xmx> ", fmt.Sprintf("Pick Xmx for %s (max RAM) — current %s", srv, curXmx))
	if err != nil {
		return err
	}
	if choice == "" {
		fmt.Println("Cancelled.")
		return nil
	}
	tag := ""
	if idx := strings.LastIndex(choice, "::"); idx >= 0 {
		tag = strings.TrimSpace(choice[idx+2:])
	} else {
		tag = strings.Fields(choice)[len(strings.Fields(choice))-1]
	}
	tag = strings.TrimPrefix(tag, "● ")
	tag = strings.TrimSpace(tag)
	xmx := tag
	if tag == "custom..." {
		fmt.Print("Enter custom Xmx (e.g. 5G, 4096M): ")
		var inp string
		fmt.Scanln(&inp)
		inp = strings.TrimSpace(inp)
		if inp == "" {
			fmt.Println("Cancelled.")
			return nil
		}
		xmx = inp
	}

	// Pick Xms via fzf (allow same + "same as Xmx")
	xmsOptions := []string{"512M", "1G", "2G", "4G", "6G", "8G", "custom...", "same as Xmx (" + xmx + ")"}
	var xmsLines []string
	for _, m := range xmsOptions {
		marker := "  "
		if m == curXms {
			marker = "● "
		}
		xmsLines = append(xmsLines, marker+m+" ::"+m)
	}
	choice2, err := runFzf(xmsLines, "Xms> ", fmt.Sprintf("Pick Xms for %s (initial RAM) — current %s", srv, curXms))
	if err != nil {
		return err
	}
	tag2 := ""
	if choice2 != "" {
		if idx := strings.LastIndex(choice2, "::"); idx >= 0 {
			tag2 = strings.TrimSpace(choice2[idx+2:])
		} else {
			tag2 = strings.Fields(choice2)[len(strings.Fields(choice2))-1]
		}
		tag2 = strings.TrimPrefix(tag2, "● ")
		tag2 = strings.TrimSpace(tag2)
		if strings.HasPrefix(tag2, "same as Xmx") {
			tag2 = xmx
		}
		if tag2 == "custom..." {
			fmt.Print("Enter custom Xms (e.g. 2G): ")
			var inp string
			fmt.Scanln(&inp)
			inp = strings.TrimSpace(inp)
			if inp != "" {
				tag2 = inp
			} else {
				tag2 = curXms
			}
		}
	} else {
		// ESC on second picker: keep current Xms
		tag2 = curXms
	}
	xms := tag2
	if xms == "" {
		xms = curXms
	}

	newFlags := updateMem(flags, xms, xmx)
	if err := writeJvmArgs(dir, newFlags); err != nil {
		return err
	}
	nxms, nxmx := parseMem(newFlags)
	fmt.Printf("Set %s RAM → Xms%s Xmx%s\n", srv, nxms, nxmx)
	return nil
}

func setupEditFlags(srv string) error {
	dir := filepath.Join(mcBase(), srv)
	flags := readJvmArgs(dir)
	fmt.Printf("Current flags for %s:\n  %s\n", srv, flags)
	fmt.Println()

	// try $EDITOR on a temp file if interactive
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor != "" && isTTY(os.Stdin) && isTTY(os.Stdout) {
		tmp, err := os.CreateTemp("", "mc-flags-*.txt")
		if err == nil {
			_, _ = tmp.WriteString(flags + "\n")
			_ = tmp.Close()
			cmd := exec.Command(editor, tmp.Name())
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err == nil {
				b, _ := os.ReadFile(tmp.Name())
				newFlags := strings.TrimSpace(string(b))
				_ = os.Remove(tmp.Name())
				if newFlags == "" {
					fmt.Println("Empty flags, not saving.")
					return nil
				}
				// normalize: if user typed a full java command, strip it
				if strings.HasPrefix(newFlags, "java ") {
					newFlags = strings.TrimPrefix(newFlags, "java ")
				}
				if idx := strings.Index(newFlags, " -jar "); idx >= 0 {
					newFlags = newFlags[:idx]
				}
				newFlags = strings.Join(strings.Fields(newFlags), " ")
				if err := writeJvmArgs(dir, newFlags); err != nil {
					return err
				}
				fmt.Println("Saved.")
				return nil
			}
			_ = os.Remove(tmp.Name())
		}
	}

	// fallback: prompt for a single line
	fmt.Println("Enter new JVM flags (or empty to cancel).")
	fmt.Printf("Current: %s\n", flags)
	fmt.Print("New flags> ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		fmt.Println("Cancelled.")
		return nil
	}
	if strings.HasPrefix(line, "java ") {
		line = strings.TrimPrefix(line, "java ")
	}
	if idx := strings.Index(line, " -jar "); idx >= 0 {
		line = line[:idx]
	}
	line = strings.Join(strings.Fields(line), " ")
	if err := writeJvmArgs(dir, line); err != nil {
		return err
	}
	fmt.Println("Saved.")
	return nil
}

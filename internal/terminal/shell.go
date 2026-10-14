package terminal

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// EnvVar is one variable a project terminal sets.
type EnvVar struct {
	Name  string
	Value string
}

// Toolchain is what a project terminal adds after PHP: PATH folders, variables, and the version lines it prints.
type Toolchain struct {
	Dirs   []string
	Env    []EnvVar
	Banner []string
}

// Environ returns base with Dirs in front of PATH and Env set.
func (t Toolchain) Environ(base []string) []string {
	out := make([]string, 0, len(base)+len(t.Env)+1)
	current := ""
	replaced := map[string]bool{}
	for _, item := range t.Env {
		replaced[strings.ToUpper(item.Name)] = true
	}
	for _, kv := range base {
		key, value, ok := strings.Cut(kv, "=")
		if !ok {
			out = append(out, kv)
			continue
		}
		if strings.EqualFold(key, "PATH") {
			current = value
			continue
		}
		if replaced[strings.ToUpper(key)] {
			continue
		}
		out = append(out, kv)
	}
	dirs := append([]string(nil), t.Dirs...)
	if current != "" {
		dirs = append(dirs, current)
	}
	out = append(out, "PATH="+strings.Join(dirs, string(os.PathListSeparator)))
	for _, item := range t.Env {
		out = append(out, item.Name+"="+item.Value)
	}
	return out
}

// WindowsProjectScript is a cmd file that puts the folder's PHP first on PATH, then the toolchains.
func WindowsProjectScript(workDir, phpDir, ini, version string, tools Toolchain) string {
	dirs := append([]string{phpDir}, tools.Dirs...)
	var b strings.Builder
	b.WriteString("@echo off\r\n")
	fmt.Fprintf(&b, "set \"PATH=%s;%%PATH%%\"\r\n", strings.Join(dirs, ";"))
	fmt.Fprintf(&b, "set \"PHPRC=%s\"\r\n", ini)
	for _, item := range tools.Env {
		fmt.Fprintf(&b, "set \"%s=%s\"\r\n", item.Name, item.Value)
	}
	fmt.Fprintf(&b, "cd /d \"%s\"\r\n", workDir)
	fmt.Fprintf(&b, "echo Phlo PHP %s\r\n", version)
	for _, line := range tools.Banner {
		fmt.Fprintf(&b, "echo %s\r\n", line)
	}
	return b.String()
}

// UnixProjectCommand opens a shell with the folder's PHP first on PATH, then the toolchains.
func UnixProjectCommand(phpDir, ini, version string, tools Toolchain) []string {
	var b strings.Builder
	b.WriteString("export PATH=")
	for _, dir := range append([]string{phpDir}, tools.Dirs...) {
		b.WriteString(strconv.Quote(dir) + ":")
	}
	b.WriteString("\"$PATH\"; ")
	fmt.Fprintf(&b, "export PHPRC=%s; ", strconv.Quote(ini))
	for _, item := range tools.Env {
		fmt.Fprintf(&b, "export %s=%s; ", item.Name, strconv.Quote(item.Value))
	}
	fmt.Fprintf(&b, "echo %s; ", strconv.Quote("Phlo PHP "+version))
	for _, line := range tools.Banner {
		fmt.Fprintf(&b, "echo %s; ", strconv.Quote(line))
	}
	b.WriteString("exec \"${SHELL:-bash}\" -i")
	return []string{"bash", "-c", b.String()}
}

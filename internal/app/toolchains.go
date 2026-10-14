package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"phlo/internal/api"
	"phlo/internal/errs"
	"phlo/internal/mongodb"
	"phlo/internal/postgres"
	"phlo/internal/proc"
	phloruntime "phlo/internal/runtime"
	"phlo/internal/terminal"
)

// clientTerminal opens psql or mongosh against the loopback database.
func (a *App) clientTerminal(kind string) (string, string, []string, error) {
	switch kind {
	case "psql":
		_, exe, err := a.toolExe("postgres")
		if err != nil {
			return "", "", nil, err
		}
		psql := phloruntime.FindSibling(exe, "psql.exe", "psql")
		if psql == "" {
			return "", "", nil, errs.New("psql is not installed next to PostgreSQL")
		}
		args := append([]string{psql}, postgres.ClientArgs(a.Config.PostgresListenPort())...)
		return "Phlo PostgreSQL", filepath.Dir(psql), args, nil
	case "mongosh":
		_, exe, err := a.toolExe("mongosh")
		if err != nil {
			return "", "", nil, err
		}
		args := []string{exe, mongodb.URI(a.Config.MongoDBListenPort())}
		return "Phlo MongoDB", filepath.Dir(exe), args, nil
	default:
		return "", "", nil, errs.New("unknown database client")
	}
}

// toolchains lists the compilers and CLIs the project terminal can use.
// limitPath skips kinds the Settings page turned off. A direct phlo command
// passes false so an installed tool still runs.
func (a *App) toolchains(limitPath bool) terminal.Toolchain {
	var out terminal.Toolchain
	for _, kind := range []string{"node", "python", "java", "gcc", "docker", "compose"} {
		if limitPath && !a.Config.ToolOnPath(kind) {
			continue
		}
		version, exe, err := a.toolExe(kind)
		if err != nil {
			if kind == "gcc" && runtime.GOOS != "windows" {
				if system, lookErr := exec.LookPath("g++"); lookErr == nil {
					out.Dirs = append(out.Dirs, filepath.Dir(system))
					out.Banner = append(out.Banner, "C++ (GCC) "+system)
				}
			}
			continue
		}
		switch kind {
		case "node":
			out.Dirs = append(out.Dirs, filepath.Dir(exe))
			prefix := a.Layout.NPMPrefix()
			_ = os.MkdirAll(prefix, 0o755)
			if runtime.GOOS == "windows" {
				out.Dirs = append(out.Dirs, prefix)
			} else {
				out.Dirs = append(out.Dirs, filepath.Join(prefix, "bin"))
			}
			out.Env = append(out.Env,
				terminal.EnvVar{Name: "npm_config_prefix", Value: prefix},
				terminal.EnvVar{Name: "npm_config_cache", Value: a.Layout.Cache("npm")},
			)
			out.Banner = append(out.Banner, "Node.js "+version)
		case "python":
			out.Dirs = append(out.Dirs, phloruntime.ToolPathDirs(kind, exe, runtime.GOOS)...)
			if runtime.GOOS == "windows" && phloruntime.FindSibling(exe, filepath.Join("Scripts", "pip.exe")) == "" {
				if shim := a.ensurePipShim(exe); shim != "" {
					out.Dirs = append(out.Dirs, filepath.Dir(shim))
				}
			}
			out.Env = append(out.Env, terminal.EnvVar{Name: "PIP_CACHE_DIR", Value: a.Layout.Cache("pip")})
			out.Banner = append(out.Banner, "Python "+version)
		case "java":
			out.Dirs = append(out.Dirs, filepath.Dir(exe))
			out.Env = append(out.Env, terminal.EnvVar{Name: "JAVA_HOME", Value: phloruntime.ToolHome(exe)})
			out.Banner = append(out.Banner, "Java "+version)
		case "gcc":
			out.Dirs = append(out.Dirs, filepath.Dir(exe))
			line := "C++ (GCC) " + version
			if runtime.GOOS == "windows" {
				line += " — add -static for an .exe that runs outside this Terminal"
			}
			out.Banner = append(out.Banner, line)
		case "docker":
			out.Dirs = append(out.Dirs, filepath.Dir(exe))
			_ = a.ensureDockerConfig()
			out.Env = append(out.Env, terminal.EnvVar{Name: "DOCKER_CONFIG", Value: a.Layout.DockerConfig()})
			out.Banner = append(out.Banner, "Docker CLI "+version)
		case "compose":
			out.Dirs = append(out.Dirs, filepath.Dir(exe))
			out.Banner = append(out.Banner, "Docker Compose "+version)
		}
	}
	return out
}

// ToolCommand resolves a passthrough binary. prefix holds arguments Phlo adds
// before the user's arguments, such as python -m pip.
func (a *App) ToolCommand(name string) (string, []string, []string, error) {
	if err := a.CheckRoot(); err != nil {
		return "", nil, nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	env := a.toolchains(false).Environ(os.Environ())
	switch name {
	case "node", "npm", "npx":
		_, node, err := a.toolExe("node")
		if err != nil {
			return "", nil, nil, err
		}
		if name == "node" {
			return node, nil, env, nil
		}
		bin := phloruntime.FindSibling(node, name+".cmd", name+".exe", name)
		if bin == "" {
			return "", nil, nil, errs.New(name + " is not installed next to Node.js")
		}
		return bin, nil, env, nil
	case "python":
		_, exe, err := a.toolExe("python")
		return exe, nil, env, err
	case "pip":
		_, exe, err := a.toolExe("python")
		if err != nil {
			return "", nil, nil, err
		}
		if runtime.GOOS == "windows" {
			if pip := phloruntime.FindSibling(exe, filepath.Join("Scripts", "pip.exe")); pip != "" {
				return pip, nil, env, nil
			}
			return exe, []string{"-m", "pip"}, env, nil
		}
		if pip := phloruntime.FindSibling(exe, "pip3", "pip"); pip != "" {
			return pip, nil, env, nil
		}
		return exe, []string{"-m", "pip"}, env, nil
	case "java":
		_, exe, err := a.toolExe("java")
		return exe, nil, env, err
	case "javac":
		_, exe, err := a.toolExe("java")
		if err != nil {
			return "", nil, nil, err
		}
		javac := phloruntime.FindSibling(exe, "javac.exe", "javac")
		if javac == "" {
			return "", nil, nil, errs.New("javac is not installed next to Java")
		}
		return javac, nil, env, nil
	case "gcc", "g++":
		_, exe, err := a.toolExe("gcc")
		if err != nil {
			if runtime.GOOS != "windows" {
				if system, lookErr := exec.LookPath(name); lookErr == nil {
					return system, nil, env, nil
				}
			}
			return "", nil, nil, err
		}
		bin := phloruntime.FindSibling(exe, name+".exe", name)
		if bin == "" {
			return "", nil, nil, errs.New(name + " is not installed next to GCC")
		}
		return bin, nil, env, nil
	case "docker":
		_, exe, err := a.toolExe("docker")
		if err != nil {
			return "", nil, nil, err
		}
		if err := a.ensureDockerConfig(); err != nil {
			return "", nil, nil, err
		}
		return exe, nil, a.toolchains(false).Environ(os.Environ()), nil
	case "psql":
		_, exe, err := a.toolExe("postgres")
		if err != nil {
			return "", nil, nil, err
		}
		psql := phloruntime.FindSibling(exe, "psql.exe", "psql")
		if psql == "" {
			return "", nil, nil, errs.New("psql is not installed next to PostgreSQL")
		}
		return psql, postgres.ClientArgs(a.Config.PostgresListenPort()), env, nil
	case "mongosh":
		_, exe, err := a.toolExe("mongosh")
		if err != nil {
			return "", nil, nil, err
		}
		return exe, []string{mongodb.URI(a.Config.MongoDBListenPort())}, env, nil
	default:
		return "", nil, nil, errs.New("unknown tool command")
	}
}

// DockerStatus asks the local engine whether it is running. Eight seconds is
// the limit; a missing engine is reported and no process is started.
func (a *App) DockerStatus(ctx context.Context) (api.Docker, error) {
	a.mu.Lock()
	_, exe, err := a.toolExe("docker")
	env := a.toolchains(false).Environ(os.Environ())
	a.mu.Unlock()
	if err != nil {
		return api.Docker{
			Installed: false,
			Hint:      "Docker CLI is not installed. Download it under Toolchains.",
		}, nil
	}
	check, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(check, exe, "version", "--format", "{{json .}}")
	cmd.Env = env
	proc.Configure(cmd)
	output, runErr := cmd.CombinedOutput()
	info := api.Docker{Installed: true, Client: exe}
	var parsed struct {
		Client *struct {
			Version string `json:"Version"`
		} `json:"Client"`
		Server *struct {
			Version string `json:"Version"`
		} `json:"Server"`
	}
	body := output
	if start := strings.IndexByte(string(output), '{'); start >= 0 {
		body = output[start:]
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Client != nil {
		info.Client = parsed.Client.Version
	}
	if runErr == nil && parsed.Server != nil && parsed.Server.Version != "" {
		info.Reachable = true
		info.Server = parsed.Server.Version
		return info, nil
	}
	info.Error = strings.TrimSpace(string(output))
	if info.Error == "" && runErr != nil {
		info.Error = runErr.Error()
	}
	info.Hint = dockerHint(runtime.GOOS, info.Error)
	return info, nil
}

func dockerHint(goos, output string) string {
	lower := strings.ToLower(output)
	if strings.Contains(lower, "permission denied") {
		return "The engine is running but your user may not use it. Add yourself to the docker group, then sign out and back in."
	}
	if goos == "windows" {
		return "No Docker engine is running. Install Docker Desktop, start it, and check again."
	}
	if strings.Contains(lower, "cannot connect") || strings.Contains(lower, "is the docker daemon running") {
		return "No Docker engine is running. Start it with sudo systemctl start docker."
	}
	return "The engine did not answer. Restart Docker Desktop or dockerd and check again."
}

func gccNote(installed bool, goos, system string) string {
	if goos == "windows" {
		return "Add -static so the .exe runs outside this Terminal."
	}
	if system != "" {
		return "System compiler: " + system
	}
	if installed {
		return ""
	}
	return "No Phlo GCC download on Linux. " + linuxGCCCommand()
}

func linuxGCCCommand() string {
	body, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "Install g++ with your distribution's package manager."
	}
	text := string(body)
	switch {
	case strings.Contains(text, "ID=fedora") || strings.Contains(text, "ID=\"fedora\"") || strings.Contains(text, "ID=rhel"):
		return "Install it with sudo dnf install gcc-c++."
	case strings.Contains(text, "ID=arch") || strings.Contains(text, "ID=\"arch\""):
		return "Install it with sudo pacman -S base-devel."
	default:
		return "Install it with sudo apt install build-essential."
	}
}

func (a *App) ensurePipShim(python string) string {
	rel, err := filepath.Rel(a.Layout.Shims(), python)
	if err != nil {
		rel = python
	}
	body := "@\"%~dp0" + rel + "\" -m pip %*\r\n"
	if err := os.MkdirAll(a.Layout.Shims(), 0o755); err != nil {
		return ""
	}
	for _, name := range []string{"pip.cmd", "pip3.cmd"} {
		if err := os.WriteFile(filepath.Join(a.Layout.Shims(), name), []byte(body), 0o644); err != nil {
			return ""
		}
	}
	return filepath.Join(a.Layout.Shims(), "pip.cmd")
}

func (a *App) ensureDockerConfig() error {
	dir := a.Layout.DockerConfig()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "config.json")
	cfg := map[string]any{}
	if body, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(body, &cfg)
	}
	if _, exe, err := a.toolExe("compose"); err == nil {
		cfg["cliPluginsExtraDirs"] = []string{filepath.Dir(exe)}
	}
	body, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o644)
}

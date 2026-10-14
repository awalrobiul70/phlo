package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"phlo/internal/app"
	"phlo/internal/dashboard"
	"phlo/internal/mariadb"
	"phlo/internal/paths"
	"phlo/internal/proc"
)

func main() {
	rootFlag, args := parseArgs(os.Args[1:])
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}
	cwd, _ := os.Getwd()
	exe, _ := os.Executable()
	root, err := paths.Resolve(rootFlag, os.Getenv("PHLO_ROOT"), cwd, exe)
	if err != nil {
		fail(err)
	}
	application, err := app.Open(root)
	if err != nil {
		fail(err)
	}
	if err := run(application, args); err != nil {
		fail(err)
	}
}

func parseArgs(args []string) (string, []string) {
	if len(args) == 0 {
		return "", args
	}
	if args[0] == "--root" && len(args) >= 2 {
		return args[1], args[2:]
	}
	if strings.HasPrefix(args[0], "--root=") {
		return strings.TrimPrefix(args[0], "--root="), args[1:]
	}
	return "", args
}

func run(application *app.App, args []string) error {
	switch args[0] {
	case "start":
		return serve(application, true)
	case "ui":
		return serve(application, false)
	case "stop":
		return stop(application)
	case "status", "scan":
		return printSites(application)
	case "version":
		return printVersion(application)
	case "php":
		return runPHP(application, args[1:])
	case "composer":
		return runComposer(application, args[1:])
	case "mysql":
		return runMySQL(application, args[1:])
	case "psql", "mongosh", "node", "npm", "npx", "python", "pip", "java", "javac", "gcc", "g++", "docker":
		return runTool(application, args[0], args[1:])
	default:
		usage()
		return fmt.Errorf("unknown command %s", args[0])
	}
}

func serve(application *app.App, startServices bool) error {
	if err := proc.Acquire(application.Layout.PhloPID()); err != nil {
		return err
	}
	ctx, stopSignal := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignal()

	var srv *http.Server
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			_, _ = application.Stop()
			if srv != nil {
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = srv.Shutdown(shutdownCtx)
			}
			_ = proc.Release(application.Layout.PhloPID())
		})
	}
	srv = &http.Server{
		Addr: fmt.Sprintf("127.0.0.1:%d", application.Config.DashboardPort),
		Handler: dashboard.Handler(application, func() { go cleanup() }, func() {
			services := application.Supervisor.Running()
			root := application.Layout.Root
			go func() {
				cleanup()
				launchUpdated(root, services)
			}()
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	if startServices {
		warn, err := application.Start()
		if err != nil {
			fmt.Fprintf(os.Stderr, "services: %s\n", err)
		} else if warn != "" {
			fmt.Fprintln(os.Stderr, warn)
		}
	}
	fmt.Printf("dashboard http://127.0.0.1:%d\n", application.Config.DashboardPort)
	go func() {
		<-ctx.Done()
		cleanup()
	}()
	err := srv.ListenAndServe()
	cleanup()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func stop(application *app.App) error {
	if err := callShutdown(application.Config.DashboardPort); err == nil {
		return nil
	}
	_, err := application.Stop()
	return err
}

func callShutdown(port int) error {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/api/shutdown", port), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Phlo", "1")
	client := &http.Client{Timeout: 8 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("shutdown returned %d", res.StatusCode)
	}
	return nil
}

func printSites(application *app.App) error {
	fmt.Printf("default php %s\n", application.Config.DefaultPHP)
	running := "stopped"
	if application.Supervisor.Running() {
		running = "running"
	}
	fmt.Printf("services %s\n", running)
	sites, err := application.ListSites()
	if err != nil {
		return err
	}
	if len(sites) == 0 {
		fmt.Println("no sites")
		return nil
	}
	for _, item := range sites {
		fmt.Printf("%s\n", item.Account)
		if item.PublicHTML != nil {
			fmt.Printf("  public_html  %s  php %s (%s)\n", item.PublicHTML.Host, item.PublicHTML.PHP, item.PublicHTML.PHPSource)
		}
		for _, sub := range item.Subdomains {
			fmt.Printf("  subdomain %s  %s  php %s (%s)\n", sub.Name, sub.Host, sub.PHP, sub.PHPSource)
		}
	}
	return nil
}

func printVersion(application *app.App) error {
	info, err := application.Release(context.Background())
	if err != nil {
		return err
	}
	fmt.Printf("Phlo %s\n", info.Version)
	if info.Title != "" {
		fmt.Println(info.Title)
	}
	if info.Message != "" {
		fmt.Println(info.Message)
	}
	if info.UpdateURL != "" {
		fmt.Println(info.UpdateURL)
	}
	if info.CanUpdate {
		fmt.Println("Open About in the dashboard and press Update.")
	}
	return nil
}

func launchUpdated(root string, services bool) {
	name := "phlo"
	if goruntime.GOOS == "windows" {
		name = "phlo.exe"
	}
	exe := filepath.Join(root, "bin", name)
	cmdName := "ui"
	if services {
		cmdName = "start"
	}
	cmd := exec.Command(exe, "--root", root, cmdName)
	cmd.Dir = root
	proc.Configure(cmd)
	_ = cmd.Start()
}

func runPHP(application *app.App, args []string) error {
	if err := application.CheckRoot(); err != nil {
		return err
	}
	exe, err := application.PHPExe(application.Config.DefaultPHP)
	if err != nil {
		return err
	}
	commandArgs := args
	ini := application.Layout.PHPIni(application.Config.DefaultPHP)
	if info, statErr := os.Stat(ini); statErr == nil && !info.IsDir() {
		commandArgs = append([]string{"-c", ini}, args...)
	}
	return runCommand(exec.Command(exe, commandArgs...))
}

func runComposer(application *app.App, args []string) error {
	if err := application.CheckRoot(); err != nil {
		return err
	}
	exe, err := application.PHPExe(application.Config.DefaultPHP)
	if err != nil {
		return err
	}
	phar := application.Layout.ComposerFile()
	if _, err := os.Stat(phar); err != nil {
		return fmt.Errorf("composer is not installed")
	}
	commandArgs := append([]string{phar}, args...)
	ini := application.Layout.PHPIni(application.Config.DefaultPHP)
	if info, statErr := os.Stat(ini); statErr == nil && !info.IsDir() {
		commandArgs = append([]string{"-c", ini}, commandArgs...)
	}
	return runCommand(exec.Command(exe, commandArgs...))
}

func runMySQL(application *app.App, args []string) error {
	exe, err := application.MySQLExe()
	if err != nil {
		return err
	}
	prefix := mariadb.ClientArgs(application.Config.MySQLListenPort())
	return runCommand(exec.Command(exe, append(prefix, args...)...))
}

func runTool(application *app.App, name string, args []string) error {
	exe, prefix, env, err := application.ToolCommand(name)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, append(prefix, args...)...)
	cmd.Env = env
	return runCommand(cmd)
}

func runCommand(cmd *exec.Cmd) error {
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		os.Exit(exitErr.ExitCode())
	}
	return err
}

func usage() {
	fmt.Fprintf(os.Stderr, `Phlo local PHP environment

Usage:
  phlo [--root path] start
  phlo [--root path] stop
  phlo [--root path] status
  phlo [--root path] version
  phlo [--root path] scan
  phlo [--root path] ui
  phlo [--root path] php [args]
  phlo [--root path] composer [args]
  phlo [--root path] mysql [args]
  phlo [--root path] psql [args]
  phlo [--root path] mongosh [args]
  phlo [--root path] node|npm|npx [args]
  phlo [--root path] python|pip [args]
  phlo [--root path] java|javac [args]
  phlo [--root path] gcc|g++ [args]
  phlo [--root path] docker [args]
`)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

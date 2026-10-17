//go:build windows

package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"

	"phlo/internal/paths"
	"phlo/internal/proc"
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	kernel32                = windows.NewLazySystemDLL("kernel32.dll")
	procCreateMutex         = kernel32.NewProc("CreateMutexW")
	procGetDpiForSystem     = user32.NewProc("GetDpiForSystem")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procGetWindowLongPtr    = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtr    = user32.NewProc("SetWindowLongPtrW")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procGetWindowRect       = user32.NewProc("GetWindowRect")
	procShowWindow          = user32.NewProc("ShowWindow")
	procLoadImage           = user32.NewProc("LoadImageW")
	procSendMessage         = user32.NewProc("SendMessageW")
	procFindWindow          = user32.NewProc("FindWindowW")
	procReleaseCapture      = user32.NewProc("ReleaseCapture")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procGetWindowThread     = user32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInput   = user32.NewProc("AttachThreadInput")
	procGetCurrentThread    = kernel32.NewProc("GetCurrentThreadId")
)

const (
	gwlStyle        = ^uintptr(15) // -16
	gwlExStyle      = ^uintptr(19) // -20
	wsCaption       = 0x00C00000
	wsSysMenu       = 0x00080000
	wsThickFrame    = 0x00040000
	wsMaximizeBox   = 0x00010000
	wsExAppWindow   = 0x00040000
	swpFrameChanged = 0x0020
	swMinimize      = 6
	swRestore       = 9
	wmSetIcon       = 0x0080
	wmNcLButtonDown = 0x00A1
	htCaption       = 2
	imageIcon       = 1
	lrLoadFromFile  = 0x00000010
)

var instance windows.Handle

func main() {
	setAppUserModelID()
	if !claimPanel() {
		focusExisting()
		return
	}
	root, err := findRoot()
	if err != nil {
		tell(err.Error())
		return
	}
	ensureDesktopShortcut(root)
	cfg := readLaunchConfig(root)
	if err := ensureServer(root, cfg.Port); err != nil {
		if dashboardBindFailed(err.Error()) {
			if openErr := openWindow(root, cfg, true); openErr != nil {
				tell(err.Error())
			}
			return
		}
		tell(err.Error())
		return
	}
	if err := openWindow(root, cfg, false); err != nil {
		_ = openBrowser(fmt.Sprintf("http://127.0.0.1:%d/panel.html", cfg.Port))
		tell(err.Error() + "\n\nOpened the panel in the browser.")
	}
}

func claimPanel() bool {
	name, err := windows.UTF16PtrFromString(`Local\PhloPanel`)
	if err != nil {
		return true
	}
	handle, _, callErr := procCreateMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if handle == 0 {
		return true
	}
	instance = windows.Handle(handle)
	return !errors.Is(callErr, windows.ERROR_ALREADY_EXISTS)
}

func focusExisting() {
	className, _ := windows.UTF16PtrFromString("webview")
	title, _ := windows.UTF16PtrFromString("Phlo")
	hwnd, _, _ := procFindWindow.Call(uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)))
	if hwnd == 0 {
		return
	}
	procShowWindow.Call(hwnd, swRestore)
	foreground, _, _ := procGetForegroundWindow.Call()
	fgThread, _, _ := procGetWindowThread.Call(foreground, 0)
	curThread, _, _ := procGetCurrentThread.Call()
	if fgThread != 0 && fgThread != curThread {
		procAttachThreadInput.Call(curThread, fgThread, 1)
		procSetForegroundWindow.Call(hwnd)
		procAttachThreadInput.Call(curThread, fgThread, 0)
		return
	}
	procSetForegroundWindow.Call(hwnd)
}

func findRoot() (string, error) {
	flag := ""
	args := os.Args[1:]
	if len(args) >= 2 && args[0] == "--root" {
		flag = args[1]
	} else if len(args) >= 1 && strings.HasPrefix(args[0], "--root=") {
		flag = strings.TrimPrefix(args[0], "--root=")
	}
	cwd, _ := os.Getwd()
	exe, _ := os.Executable()
	return paths.Resolve(flag, os.Getenv("PHLO_ROOT"), cwd, exe)
}

func ensureServer(root string, port int) error {
	if healthy(port) {
		return nil
	}
	if portTaken(port) {
		return fmt.Errorf("127.0.0.1:%d is already in use", port)
	}
	exe := filepath.Join(root, "bin", "phlo.exe")
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("Build Phlo first: go build -o bin\\phlo.exe .\\cmd\\phlo")
	}
	logPath := filepath.Join(root, "var", "log", "panel-start.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "--root", root, "start")
	cmd.Dir = root
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	proc.Configure(cmd)
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return err
	}
	_ = logFile.Close()
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			if healthy(port) {
				return nil
			}
		case <-done:
			if healthy(port) {
				return nil
			}
			return startFailed(logPath)
		case <-timer.C:
			return startFailed(logPath)
		}
	}
}

func startFailed(logPath string) error {
	detail := tail(logPath)
	if detail == "" {
		return fmt.Errorf("Phlo did not start. See %s", logPath)
	}
	return fmt.Errorf("Phlo did not start.\n\n%s", detail)
}

func tail(path string) string {
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(string(body))
	if len(text) > 700 {
		text = text[len(text)-700:]
	}
	return text
}

func healthy(port int) bool {
	client := &http.Client{Timeout: time.Second}
	res, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/health", port))
	if err != nil {
		return false
	}
	defer res.Body.Close()
	var body struct {
		OK bool `json:"ok"`
	}
	return json.NewDecoder(res.Body).Decode(&body) == nil && body.OK
}

func applyDashboardPort(root string, port int) (string, error) {
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("dashboardPort must be between 1 and 65535")
	}
	current := readLaunchConfig(root)
	if port == current.Port && healthy(port) {
		return fmt.Sprintf("http://127.0.0.1:%d/panel.html", port), nil
	}
	if portTaken(port) {
		return "", fmt.Errorf("127.0.0.1:%d is already in use. Choose another port.", port)
	}
	if port != current.Port {
		if err := dashboardPortAllowed(root, port); err != nil {
			return "", err
		}
	}
	if healthy(current.Port) {
		if err := postShutdown(current.Port); err != nil {
			return "", err
		}
		deadline := time.Now().Add(60 * time.Second)
		for healthy(current.Port) && time.Now().Before(deadline) {
			time.Sleep(200 * time.Millisecond)
		}
		if healthy(current.Port) {
			return "", fmt.Errorf("Phlo is still running on 127.0.0.1:%d", current.Port)
		}
		time.Sleep(400 * time.Millisecond)
	}
	if port != current.Port {
		if err := saveDashboardPort(root, port); err != nil {
			return "", err
		}
	}
	var err error
	for i := 0; i < 15; i++ {
		err = ensureServer(root, port)
		if err == nil || !strings.Contains(err.Error(), "already running") {
			break
		}
		time.Sleep(400 * time.Millisecond)
	}
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("http://127.0.0.1:%d/panel.html", port), nil
}

func postShutdown(port int) error {
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/api/shutdown", port), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Phlo", "1")
	client := &http.Client{Timeout: 20 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("could not stop the dashboard on 127.0.0.1:%d", port)
	}
	return nil
}

func openWindow(root string, cfg launchConfig, choosePort bool) error {
	dpi, _, _ := procGetDpiForSystem.Call()
	width, height := fit(px(440, dpi), px(640, dpi))
	_ = os.MkdirAll(filepath.Join(root, "var", "webview"), 0o755)
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  filepath.Join(root, "var", "webview"),
		WindowOptions: webview2.WindowOptions{
			Title:  "Phlo",
			Width:  width,
			Height: height,
			Center: true,
		},
	})
	if w == nil {
		return fmt.Errorf("The Phlo window needs the Microsoft Edge WebView2 runtime.")
	}
	defer w.Destroy()
	hwnd := uintptr(w.Window())
	w.SetSize(int(px(380, dpi)), int(px(520, dpi)), webview2.HintMin)
	stripCaption(hwnd)
	setWindowIcon(hwnd, filepath.Join(root, "ui", "logo.ico"))
	_ = w.Bind("phloMinimize", func() { procShowWindow.Call(hwnd, swMinimize) })
	_ = w.Bind("phloClose", func() { w.Destroy() })
	_ = w.Bind("phloDrag", func() { dragWindow(hwnd) })
	_ = w.Bind("phloOpen", func(raw string) error {
		link, err := localURL(cfg, raw)
		if err != nil {
			return err
		}
		return openBrowser(link)
	})
	_ = w.Bind("phloSetDashboardPort", func(port int) {
		go func() {
			url, err := applyDashboardPort(root, port)
			message := ""
			if err != nil {
				message = err.Error()
				url = ""
			}
			target, _ := json.Marshal(url)
			problem, _ := json.Marshal(message)
			w.Dispatch(func() {
				defer func() { _ = recover() }()
				w.Eval(fmt.Sprintf("window.phloPortDone && window.phloPortDone(%s, %s)", target, problem))
			})
		}()
	})
	if choosePort {
		w.SetHtml(recoveryPage(root, cfg.Port))
	} else {
		w.Navigate(fmt.Sprintf("http://127.0.0.1:%d/panel.html", cfg.Port))
	}
	w.Run()
	return nil
}

func recoveryPage(root string, blocked int) string {
	suggested := suggestDashboardPort(root, blocked)
	value := ""
	if suggested > 0 {
		value = fmt.Sprintf(` value="%d"`, suggested)
	}
	logo := ""
	if body, err := os.ReadFile(filepath.Join(root, "ui", "logo.svg")); err == nil {
		logo = `<img class="logo" alt="" src="data:image/svg+xml;base64,` + base64.StdEncoding.EncodeToString(body) + `">`
	}
	problem := fmt.Sprintf("127.0.0.1:%d is already in use, so the dashboard could not start. The program using that port was left running. Choose another port and Phlo will start there. Other service ports stay as they are.", blocked)
	page := strings.ReplaceAll(recoveryHTML, "__LOGO__", logo)
	page = strings.ReplaceAll(page, "__PROBLEM__", html.EscapeString(problem))
	page = strings.ReplaceAll(page, "__VALUE__", value)
	return page
}

const recoveryHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Phlo</title>
  <style>
    :root { color: #17241e; background: #f3efe7; font-family: "Segoe UI", "Aptos", sans-serif; }
    * { box-sizing: border-box; }
    body { margin: 0; min-height: 100vh; display: flex; flex-direction: column; }
    h1, p { margin: 0; }
    button, input { font: inherit; }
    .bar { display: flex; align-items: center; gap: 0.75rem; padding: 0.85rem 0.9rem 0.85rem 1rem; color: #f7fbf8; background: linear-gradient(125deg, #10281f 0%, #0e5a3e 58%, #1c8a5a 100%); user-select: none; }
    .logo { width: 2.5rem; height: 2.5rem; border-radius: 0.7rem; }
    .mark { font-size: 0.78rem; font-weight: 700; letter-spacing: 0.14em; text-transform: uppercase; opacity: 0.82; }
    h1 { font-size: 1.25rem; font-weight: 650; }
    .chrome { margin-left: auto; display: flex; gap: 0.25rem; }
    .chrome[hidden] { display: none; }
    .chrome button { width: 2.1rem; height: 1.85rem; border: 0; border-radius: 8px; background: transparent; color: #f7fbf8; font-size: 1.15rem; cursor: pointer; }
    .chrome button:hover { background: rgba(255,255,255,0.14); }
    #close:hover { background: #c4453c; }
    main { padding: 1rem; }
    .banner { margin: 0 0 0.9rem; padding: 0.7rem 0.8rem; border-radius: 12px; background: #f8ecec; color: #8d2a2a; }
    #note { background: #f8ecec; }
    form { display: flex; align-items: center; gap: 0.45rem; }
    label { color: #5d6b63; font-size: 0.85rem; }
    input { width: 6rem; border: 1px solid #d5ddd8; border-radius: 10px; padding: 0.45rem 0.55rem; background: #fff; }
    button[type="submit"] { border: 0; border-radius: 12px; background: #0f6a45; color: white; padding: 0.5rem 0.8rem; font-weight: 650; cursor: pointer; }
    button[type="submit"]:disabled { opacity: 0.7; cursor: wait; }
  </style>
</head>
<body>
  <header class="bar">
    __LOGO__
    <div>
      <p class="mark">Phlo</p>
      <h1>localDev</h1>
    </div>
    <div class="chrome" hidden>
      <button type="button" id="min" aria-label="Minimize">&#8211;</button>
      <button type="button" id="close" aria-label="Close">&#215;</button>
    </div>
  </header>
  <main>
    <p class="banner">__PROBLEM__</p>
    <p id="note" class="banner" role="alert" hidden></p>
    <form id="port-form">
      <label for="port">Dashboard port</label>
      <input id="port" type="number" min="1" max="65535" required autofocus__VALUE__>
      <button type="submit">Use this port</button>
    </form>
  </main>
  <script>
    function whenBound(fn) {
      var tries = 0;
      (function tick() {
        if (window.phloSetDashboardPort || tries++ > 50) { fn(); return; }
        setTimeout(tick, 50);
      })();
    }
    whenBound(function () {
      var chrome = document.querySelector(".chrome");
      if (window.phloClose) {
        chrome.hidden = false;
        document.querySelector("#min").addEventListener("click", function () { window.phloMinimize(); });
        document.querySelector("#close").addEventListener("click", function () { window.phloClose(); });
        document.querySelector(".bar").addEventListener("mousedown", function (event) {
          if (event.button !== 0 || event.target.closest("button")) return;
          window.phloDrag();
        });
      }
      document.querySelector("#port-form").addEventListener("submit", function (event) {
        event.preventDefault();
        var input = document.querySelector("#port");
        var button = document.querySelector("button[type=submit]");
        var note = document.querySelector("#note");
        var port = Number(input.value);
        note.hidden = true;
        if (!Number.isInteger(port) || port < 1 || port > 65535) {
          note.textContent = "The dashboard port must be from 1 to 65535.";
          note.hidden = false;
          return;
        }
        if (!window.phloSetDashboardPort) {
          note.textContent = "The window is still starting. Try again.";
          note.hidden = false;
          return;
        }
        button.disabled = true;
        button.textContent = "Starting";
        new Promise(function (resolve, reject) {
          window.phloPortDone = function (url, err) { if (err) reject(err); else resolve(url); };
          window.phloSetDashboardPort(port).catch(reject);
        }).then(function (url) {
          location.href = url;
        }).catch(function (err) {
          note.textContent = (typeof err === "string" && err) || (err && err.message) || "Could not change the dashboard port.";
          note.hidden = false;
          button.disabled = false;
          button.textContent = "Use this port";
        });
      });
    });
  </script>
</body>
</html>
`

func stripCaption(hwnd uintptr) {
	style, _, _ := procGetWindowLongPtr.Call(hwnd, gwlStyle)
	style &^= wsCaption | wsSysMenu | wsThickFrame | wsMaximizeBox
	procSetWindowLongPtr.Call(hwnd, gwlStyle, style)
	ex, _, _ := procGetWindowLongPtr.Call(hwnd, gwlExStyle)
	procSetWindowLongPtr.Call(hwnd, gwlExStyle, ex|wsExAppWindow)
	var rect struct {
		Left, Top, Right, Bottom int32
	}
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	procSetWindowPos.Call(hwnd, 0, uintptr(rect.Left), uintptr(rect.Top), uintptr(rect.Right-rect.Left), uintptr(rect.Bottom-rect.Top), swpFrameChanged)
}

func dragWindow(hwnd uintptr) {
	procReleaseCapture.Call()
	procSendMessage.Call(hwnd, wmNcLButtonDown, htCaption, 0)
}

func setWindowIcon(hwnd uintptr, path string) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	bigSize, _, _ := procGetSystemMetrics.Call(11)   // SM_CXICON
	smallSize, _, _ := procGetSystemMetrics.Call(49) // SM_CXSMICON
	if bigSize == 0 {
		bigSize = 32
	}
	if smallSize == 0 {
		smallSize = 16
	}
	big, _, _ := procLoadImage.Call(0, uintptr(unsafe.Pointer(p)), imageIcon, bigSize, bigSize, lrLoadFromFile)
	small, _, _ := procLoadImage.Call(0, uintptr(unsafe.Pointer(p)), imageIcon, smallSize, smallSize, lrLoadFromFile)
	if big != 0 {
		procSendMessage.Call(hwnd, wmSetIcon, 1, big)
	}
	if small != 0 {
		procSendMessage.Call(hwnd, wmSetIcon, 0, small)
	}
}

func px(n, dpi uintptr) uint {
	if dpi < 96 {
		dpi = 96
	}
	return uint((n*dpi + 48) / 96)
}

func fit(width, height uint) (uint, uint) {
	sw, _, _ := procGetSystemMetrics.Call(0)
	sh, _, _ := procGetSystemMetrics.Call(1)
	if sw > 200 && width > uint(sw)*9/10 {
		width = uint(sw) * 9 / 10
	}
	if sh > 200 && height > uint(sh)*85/100 {
		height = uint(sh) * 85 / 100
	}
	return width, height
}

func openBrowser(raw string) error {
	cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", raw)
	proc.Configure(cmd)
	return cmd.Start()
}

func tell(text string) {
	body, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	caption, err := windows.UTF16PtrFromString("Phlo")
	if err != nil {
		return
	}
	_, _ = windows.MessageBox(0, body, caption, windows.MB_OK|windows.MB_ICONERROR)
}

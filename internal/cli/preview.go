package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/ShiroDoromoto/crofty/internal/hugobin"
	"github.com/ShiroDoromoto/crofty/internal/project"
	"github.com/ShiroDoromoto/crofty/internal/theme"
)

// previewState records a running local preview so a later, separate crofty
// process can find and stop it (`crofty preview stop`) or report on it
// (`crofty preview status`). It lives at .crofty/preview.json — machine state,
// never content, and gitignored — so it can't ride along to deploy.
//
// It carries both the wrapper's pid (the `crofty preview` process, which owns
// the graceful teardown) and hugo's pid (the child), so stop can end the whole
// preview even if the wrapper already died without cleaning up.
type previewState struct {
	CroftyPID int    `json:"croftyPid"`
	HugoPID   int    `json:"hugoPid"`
	Port      int    `json:"port"`
	URL       string `json:"url"`
	StartedAt string `json:"startedAt"`           // RFC3339
	TimeoutAt string `json:"timeoutAt,omitempty"` // RFC3339; empty when no auto-stop
}

// previewStatePath is the per-project preview state file.
func previewStatePath(proj *project.Project) string {
	return filepath.Join(proj.Root, project.MarkerDir, "preview.json")
}

func readPreviewState(path string) (*previewState, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var st previewState
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func writePreviewState(path string, st *previewState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func removePreviewState(path string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		// A stale state file is harmless; don't fail a command over it.
		fmt.Fprintf(os.Stderr, "crofty: could not remove %s: %v\n", path, err)
	}
}

// runPreview routes `crofty preview` and its two management subcommands. A bare
// `crofty preview` (or one with flags like --timeout) starts the server; `stop`
// and `status` operate on whatever is already running.
func runPreview(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "stop":
			return runPreviewStop(args[1:])
		case "status":
			return runPreviewStatus(args[1:])
		}
	}
	return runPreviewStart(args)
}

// runPreviewStart serves the site locally with Hugo's dev server so anyone can
// see their site in a browser before connecting any account — the first win that
// needs no Cloudflare, no keys, nothing but the folder on this machine. It
// blocks, streaming Hugo's output, until the user presses Control-C, until
// --timeout elapses, or until `crofty preview stop` ends it from elsewhere.
//
// While it runs it records .crofty/preview.json so it can be stopped without a
// terminal to Control-C — the case that matters when an AI backgrounds it to
// keep working and would otherwise leave it running forever.
func runPreviewStart(args []string) error {
	fs := flag.NewFlagSet("preview", flag.ContinueOnError)
	timeout := fs.Duration("timeout", 30*time.Minute,
		"auto-stop after this long (0 = run until stopped) — a backstop so a backgrounded preview never lingers")
	port := fs.Int("port", 1313, "local port to serve on")
	fs.Usage = func() {
		fmt.Println("crofty preview — see your site in a browser (local, no account)")
		fmt.Println("\nUsage:")
		fmt.Println("  crofty preview [--timeout 30m] [--port 1313]   # start (blocks until Control-C / timeout)")
		fmt.Println("  crofty preview stop                            # stop a preview started in the background")
		fmt.Println("  crofty preview status [--json]                 # is one running? where?")
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	proj, err := findProject()
	if err != nil {
		return err
	}

	hugoExe, err := hugobin.Resolve()
	if err != nil {
		return err
	}

	// Singleton: at most one preview per project. If one is already running,
	// reap it before starting — so a re-run heals a forgotten preview instead of
	// piling a second server onto a new port.
	statePath := previewStatePath(proj)
	if old, _ := readPreviewState(statePath); old != nil && processAlive(old.CroftyPID) {
		fmt.Printf("A preview is already running (pid %d, %s). Stopping it first.\n", old.CroftyPID, old.URL)
		stopPreviewState(old)
	}
	removePreviewState(statePath)

	themeDst := filepath.Join(proj.ThemesDir(), "crofty")
	if err := theme.Materialize(themeDst); err != nil {
		return fmt.Errorf("writing bundled theme: %w", err)
	}

	fmt.Println("Starting a local preview of your site.")
	fmt.Printf("Open http://localhost:%d in your web browser.\n", *port)
	fmt.Println("Edits to content and styles reload automatically. If a change to hugo.yaml")
	fmt.Println("(or, rarely, a stylesheet) doesn't show, stop with Control-C and run this again.")
	if *timeout > 0 {
		fmt.Printf("When you're done, press Control-C (or run 'crofty preview stop'). It also\n")
		fmt.Printf("auto-stops after %s so a forgotten preview never lingers.\n", *timeout)
	} else {
		fmt.Println("When you're done looking, press Control-C here (or run 'crofty preview stop').")
	}
	fmt.Println()

	// --disableFastRender makes every edit trigger a full rebuild: a touch slower,
	// but edits (including assets) reliably appear — important when an agent writes
	// a file and checks the result, where a stale render reads as a real failure.
	cmd := exec.Command(hugoExe, "server",
		"--source", proj.Root,
		"--themesDir", proj.ThemesDir(),
		"--theme", "crofty",
		"--disableFastRender",
		"--port", strconv.Itoa(*port),
	)
	cmd.Dir = proj.Root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting hugo: %w", err)
	}

	now := time.Now()
	st := &previewState{
		CroftyPID: os.Getpid(),
		HugoPID:   cmd.Process.Pid,
		Port:      *port,
		URL:       fmt.Sprintf("http://localhost:%d/", *port),
		StartedAt: now.Format(time.RFC3339),
	}
	if *timeout > 0 {
		st.TimeoutAt = now.Add(*timeout).Format(time.RFC3339)
	}
	if err := writePreviewState(statePath, st); err != nil {
		// State is only for out-of-band stop; a preview without it is still usable
		// (Control-C works), so warn rather than abort.
		fmt.Fprintf(os.Stderr, "crofty: could not record preview state: %v\n", err)
	}
	defer removePreviewState(statePath)

	// A Control-C or an external `crofty preview stop` (SIGTERM) is the normal way
	// to end a preview, not a crofty failure. Catch both, tear hugo down, exit 0.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	var timeoutCh <-chan time.Time
	if *timeout > 0 {
		t := time.NewTimer(*timeout)
		defer t.Stop()
		timeoutCh = t.C
	}

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	select {
	case <-sigCh:
		signalTerminate(cmd.Process.Pid)
		<-waitCh
		return nil
	case <-timeoutCh:
		fmt.Printf("\nPreview auto-stopped after %s. Run 'crofty preview' again to restart.\n", *timeout)
		signalTerminate(cmd.Process.Pid)
		<-waitCh
		return nil
	case err := <-waitCh:
		// hugo exited on its own — a bind failure, a crash, or an external kill.
		if err != nil {
			return fmt.Errorf("preview stopped: %w", err)
		}
		return nil
	}
}

// runPreviewStop ends the preview running for this project. It is idempotent:
// calling it when nothing is running is a success, so an agent can call it
// unconditionally when it's done showing the author their site.
func runPreviewStop(args []string) error {
	fs := flag.NewFlagSet("preview stop", flag.ContinueOnError)
	fs.Usage = func() { fmt.Println("crofty preview stop — stop the local preview for this project") }
	if err := fs.Parse(args); err != nil {
		return err
	}

	proj, err := findProject()
	if err != nil {
		return err
	}
	statePath := previewStatePath(proj)

	st, _ := readPreviewState(statePath)
	if st == nil || !processAlive(st.CroftyPID) {
		removePreviewState(statePath)
		fmt.Println("No preview is running for this project.")
		return nil
	}

	fmt.Printf("Stopping preview (pid %d, %s)...\n", st.CroftyPID, st.URL)
	stopPreviewState(st)
	removePreviewState(statePath)
	fmt.Println("Stopped.")
	return nil
}

// stopPreviewState ends a recorded preview. It asks the wrapper to stop first
// (SIGTERM, so it tears hugo down and cleans up), then makes sure hugo itself is
// gone even if the wrapper had already died — the belt-and-braces that keeps a
// hugo server from being orphaned.
func stopPreviewState(st *previewState) {
	terminatePID(st.CroftyPID)
	terminatePID(st.HugoPID)
}

// terminatePID sends SIGTERM to a live pid, waits briefly for it to exit, then
// SIGKILLs if it's still there. A dead or zero pid is a no-op.
func terminatePID(pid int) {
	if pid <= 0 || !processAlive(pid) {
		return
	}
	signalTerminate(pid)
	for i := 0; i < 30; i++ {
		if !processAlive(pid) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	signalKill(pid)
}

// previewReport is the machine-readable answer to "is a preview running?".
type previewReport struct {
	Running   bool   `json:"running"`
	URL       string `json:"url,omitempty"`
	PID       int    `json:"pid,omitempty"`
	Port      int    `json:"port,omitempty"`
	StartedAt string `json:"startedAt,omitempty"`
	TimeoutAt string `json:"timeoutAt,omitempty"`
}

// runPreviewStatus reports whether a preview is running for this project, so an
// agent can decide whether to start one, reuse the running URL, or stop it —
// the live-state surface `crofty agent` points at.
func runPreviewStatus(args []string) error {
	fs := flag.NewFlagSet("preview status", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "emit the status as JSON")
	fs.Usage = func() { fmt.Println("crofty preview status — is a local preview running for this project?") }
	if err := fs.Parse(args); err != nil {
		return err
	}

	proj, err := findProject()
	if err != nil {
		return err
	}
	statePath := previewStatePath(proj)

	st, _ := readPreviewState(statePath)
	rep := previewReport{}
	if st != nil && processAlive(st.CroftyPID) {
		rep = previewReport{
			Running:   true,
			URL:       st.URL,
			PID:       st.CroftyPID,
			Port:      st.Port,
			StartedAt: st.StartedAt,
			TimeoutAt: st.TimeoutAt,
		}
	} else if st != nil {
		// Recorded but the process is gone (e.g. SIGKILL never cleaned up). Tidy
		// the stale file so the next status is fast and honest.
		removePreviewState(statePath)
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}

	if !rep.Running {
		fmt.Println("No preview is running for this project.")
		fmt.Println("Start one with 'crofty preview'.")
		return nil
	}
	fmt.Printf("Preview is running at %s (pid %d).\n", rep.URL, rep.PID)
	if rep.TimeoutAt != "" {
		fmt.Printf("It auto-stops at %s. Stop it now with 'crofty preview stop'.\n", rep.TimeoutAt)
	} else {
		fmt.Println("Stop it with 'crofty preview stop'.")
	}
	return nil
}

package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
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

// previewLogPath is where a detached preview's output goes, since it has no
// terminal to stream to. Machine-local and gitignored, like preview.json.
func previewLogPath(proj *project.Project) string {
	return filepath.Join(proj.Root, project.MarkerDir, "preview.log")
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
	detach := fs.Bool("detach", false,
		"start the preview in the background and return once it answers — stop it with 'crofty preview stop'")
	fs.Usage = func() {
		fmt.Println("crofty preview — see your site in a browser (local, no account)")
		fmt.Println("\nUsage:")
		fmt.Println("  crofty preview [--timeout 30m] [--port 1313]   # start (blocks until Control-C / timeout)")
		fmt.Println("  crofty preview --detach                        # start in the background, return once it serves")
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

	if *detach {
		return startDetachedPreview(proj, *port, *timeout)
	}

	hugoExe, err := hugobin.Resolve()
	if err != nil {
		return err
	}

	statePath := previewStatePath(proj)
	reapExistingPreview(statePath)

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

// reapExistingPreview enforces the singleton: at most one preview per project.
// If one is already running it is stopped before the new one starts — so a
// re-run heals a forgotten preview instead of piling a second server onto a new
// port. Both the blocking and the detached start go through here, so a detached
// re-run can take back the port the previous one holds.
func reapExistingPreview(statePath string) {
	old, _ := readPreviewState(statePath)
	wrapper, hugo := previewAlive(old)
	switch {
	case wrapper:
		fmt.Printf("A preview is already running (pid %d, %s). Stopping it first.\n", old.CroftyPID, old.URL)
		stopPreviewState(old)
	case hugo:
		// The wrapper died without taking hugo with it. Collect the orphan here
		// rather than deleting the state and losing the pid that could end it.
		fmt.Printf("An abandoned preview server is still running (pid %d, %s). Stopping it first.\n", old.HugoPID, old.URL)
		stopPreviewState(old)
	}
	removePreviewState(statePath)
}

// startDetachedPreview runs `crofty preview` again as a detached process and
// returns as soon as the server answers on its port. Everything a preview needs
// to be stoppable — preview.json, the auto-stop timer, the singleton reap — is
// the blocking path's, so this reuses it rather than reimplementing it: crofty
// re-executes itself without --detach, in its own session/process group so a
// Control-C in this terminal (or the shell exiting) can't take it down.
//
// It exists because the alternative is every agent inventing its own way to
// background a blocking command — `start`, `Start-Process`, `&` — which is where
// the fragile, platform-specific shell tricks come from.
func startDetachedPreview(proj *project.Project, port int, timeout time.Duration) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding the crofty binary to re-run in the background: %w", err)
	}

	// Fail before forking rather than leaving the author to read a log to learn
	// hugo is missing.
	if _, err := hugobin.Resolve(); err != nil {
		return err
	}

	// Take back the port a previous preview of this project holds, then insist the
	// port is actually free. Readiness below is "something answers on this port",
	// which a squatter would satisfy while our hugo dies unnoticed — so the
	// squatter has to be ruled out here, where we can name the problem.
	reapExistingPreview(previewStatePath(proj))
	if err := previewPortFree(port); err != nil {
		return err
	}

	logPath := previewLogPath(proj)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("opening the preview log: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(exe, "preview",
		"--port", strconv.Itoa(port),
		"--timeout", timeout.String(),
	)
	cmd.Dir = proj.Root
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = detachedSysProcAttr()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting the background preview: %w", err)
	}
	pid := cmd.Process.Pid
	// Nothing will Wait for this child: it outlives us on purpose.
	_ = cmd.Process.Release()

	// Wait for evidence, not for a clock: the port answering means hugo bound it
	// and the author can open the URL. If the child dies first (a taken port, a
	// broken hugo), say so with the log instead of reporting a preview that isn't.
	url := fmt.Sprintf("http://localhost:%d/", port)
	deadline := time.Now().Add(30 * time.Second)
	for {
		alive := processAlive(pid)
		if alive && previewPortAnswers(port) {
			break
		}
		if !alive {
			return fmt.Errorf("the background preview exited before it served anything:\n%s\nFull log: %s",
				indentedLogTail(logPath, 10), logPath)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the background preview did not answer on port %d within 30s (it is still running as pid %d).\nStop it with 'crofty preview stop'; its log is %s",
				port, pid, logPath)
		}
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Printf("Preview is running in the background at %s (pid %d).\n", url, pid)
	fmt.Printf("Output goes to %s.\n", logPath)
	if timeout > 0 {
		fmt.Printf("Stop it with 'crofty preview stop'. It also auto-stops after %s.\n", timeout)
	} else {
		fmt.Println("Stop it with 'crofty preview stop'.")
	}
	return nil
}

// previewPortFree reports whether the preview port can still be bound — and if
// not, says so as a choice the author can act on (pick another port, or stop
// whatever holds this one) rather than as a hugo error buried in a log.
func previewPortFree(port int) error {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("port %d is already in use by something else on this machine.\nServe on another port with 'crofty preview --detach --port %d', or stop what's holding it", port, port+1)
	}
	return ln.Close()
}

// previewPortAnswers reports whether something is listening on the preview port
// on this machine — the readiness signal for a detached start, trustworthy only
// because previewPortFree ruled out a squatter before hugo was started.
func previewPortAnswers(port int) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// indentedLogTail returns the last n lines of the preview log, indented, so a
// failed detached start can show why without making the caller open a file.
func indentedLogTail(path string, n int) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "  (no log was written)"
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	if len(lines) == 1 && lines[0] == "" {
		return "  (the log is empty)"
	}
	return "  " + strings.Join(lines, "\n  ")
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
	wrapper, hugo := previewAlive(st)
	if !wrapper && !hugo {
		removePreviewState(statePath)
		fmt.Println("No preview is running for this project.")
		return nil
	}

	if wrapper {
		fmt.Printf("Stopping preview (pid %d, %s)...\n", st.CroftyPID, st.URL)
	} else {
		fmt.Printf("Stopping an abandoned preview server (pid %d, %s)...\n", st.HugoPID, st.URL)
	}
	stopPreviewState(st)
	removePreviewState(statePath)
	fmt.Println("Stopped.")
	return nil
}

// previewAlive answers, for a recorded preview, which of its two processes are
// still running. Both halves matter: the wrapper owns the graceful teardown and
// the auto-stop timer, but hugo is the thing holding the port, and it outlives a
// wrapper that was killed rather than asked to stop. Treating "wrapper gone" as
// "nothing is running" is what used to strand a hugo server for good — the state
// file was deleted, and with it the only record of the pid to kill.
func previewAlive(st *previewState) (wrapper, hugo bool) {
	if st == nil {
		return false, false
	}
	return processIs(st.CroftyPID, croftyExeName()), processIs(st.HugoPID, "hugo")
}

// croftyExeName is the name the wrapper runs under — this binary's own, since a
// detached preview is this binary re-executed. Reading it rather than hardcoding
// "crofty" means a renamed binary still recognizes the preview it started.
func croftyExeName() string {
	exe, err := os.Executable()
	if err != nil {
		return "crofty"
	}
	return strings.ToLower(filepath.Base(exe))
}

// previewExpired reports whether a recorded preview is past the auto-stop time
// it was started with. An empty TimeoutAt means `--timeout 0` — the author asked
// for a preview that runs until they stop it, and that wish outlives the wrapper.
// An unparseable time is treated as no deadline: crofty does not kill on a guess.
func previewExpired(st *previewState, now time.Time) bool {
	if st == nil || st.TimeoutAt == "" {
		return false
	}
	deadline, err := time.Parse(time.RFC3339, st.TimeoutAt)
	if err != nil {
		return false
	}
	return !now.Before(deadline)
}

// sweepExpiredPreview is the backstop for a preview whose wrapper died before its
// --timeout could fire. That timer lives in the wrapper's memory, so killing the
// wrapper kills the promise with it, and hugo would serve until the machine went
// down. The deadline itself, though, is written in preview.json — so any later
// crofty run in this project can honour it.
//
// Every project command goes through findProject, which calls this: the sweep
// costs one small file read, and in exchange no preview outlives its deadline for
// longer than it takes the author (or their AI) to run crofty again. It says what
// it did on stderr, since stdout may be JSON somebody is parsing.
func sweepExpiredPreview(proj *project.Project) {
	statePath := previewStatePath(proj)
	st, err := readPreviewState(statePath)
	if err != nil || st == nil {
		return
	}
	wrapper, hugo := previewAlive(st)
	if !wrapper && !hugo {
		// Nothing left to stop; the record is litter.
		removePreviewState(statePath)
		return
	}
	if !previewExpired(st, time.Now()) {
		return
	}
	fmt.Fprintf(os.Stderr, "crofty: stopped a preview that was past its auto-stop time (%s, %s).\n", st.TimeoutAt, st.URL)
	stopPreviewState(st)
	removePreviewState(statePath)
}

// stopPreviewState ends a recorded preview. It asks the wrapper to stop first
// (SIGTERM, so it tears hugo down and cleans up), then makes sure hugo itself is
// gone even if the wrapper had already died — the belt-and-braces that keeps a
// hugo server from being orphaned.
func stopPreviewState(st *previewState) {
	terminatePID(st.CroftyPID, croftyExeName())
	terminatePID(st.HugoPID, "hugo")
}

// terminatePID sends SIGTERM to a live pid, waits briefly for it to exit, then
// SIGKILLs if it's still there. A dead or zero pid is a no-op.
//
// want names the program the pid is supposed to be. A recorded pid can be stale
// — the process died and the OS handed the number to something unrelated — so a
// pid alone is never licence to kill. When the name can't be read, crofty leaves
// the process alone: killing a stranger is worse than leaving a hugo running,
// and the wrapper exits on its own once its hugo is gone anyway.
func terminatePID(pid int, want string) {
	if !processIs(pid, want) {
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

// processIs reports whether pid is a live process whose program name contains
// want — the guard against a recycled pid.
func processIs(pid int, want string) bool {
	if pid <= 0 || !processAlive(pid) {
		return false
	}
	name, err := processName(pid)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(name), want)
}

// previewReport is the machine-readable answer to "is a preview running?".
//
// Abandoned marks the case where hugo is still serving but the crofty process
// that supervised it is gone: the site is up, and nothing will ever auto-stop it,
// because the timer lived in that process. It is still running — reporting it as
// stopped would hide the one thing the reader needs to act on.
type previewReport struct {
	Running   bool   `json:"running"`
	Abandoned bool   `json:"abandoned,omitempty"`
	URL       string `json:"url,omitempty"`
	PID       int    `json:"pid,omitempty"`
	HugoPID   int    `json:"hugoPid,omitempty"`
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
	wrapper, hugo := previewAlive(st)
	rep := previewReport{}
	switch {
	case wrapper || hugo:
		rep = previewReport{
			Running:   true,
			Abandoned: !wrapper,
			URL:       st.URL,
			Port:      st.Port,
			StartedAt: st.StartedAt,
		}
		if wrapper {
			rep.PID = st.CroftyPID
			// The auto-stop timer lives in the wrapper, so it is only a promise
			// while the wrapper is there to keep it.
			rep.TimeoutAt = st.TimeoutAt
		}
		if hugo {
			rep.HugoPID = st.HugoPID
		}
	case st != nil:
		// Recorded but both processes are gone (e.g. SIGKILL never cleaned up).
		// Tidy the stale file so the next status is fast and honest.
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
	if rep.Abandoned {
		fmt.Printf("A preview server is still serving %s (pid %d), but the crofty process\n", rep.URL, rep.HugoPID)
		fmt.Println("that supervised it is gone, so it will not auto-stop on its own.")
		fmt.Println("Stop it with 'crofty preview stop'.")
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

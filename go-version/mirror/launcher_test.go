package mirror

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultLauncherPathsAndOpenVPNBin(t *testing.T) {
	home := t.TempDir()
	winPath := filepath.Join(home, "Program Files", "OpenVPN", "bin")
	if err := os.MkdirAll(winPath, 0o755); err != nil {
		t.Fatal(err)
	}
	openvpnExe := filepath.Join(winPath, "openvpn.exe")
	if err := os.WriteFile(openvpnExe, []byte("bin"), 0o644); err != nil {
		t.Fatal(err)
	}
	l := NewDefaultLauncher(home, "windows", map[string]string{"ProgramFiles": filepath.Join(home, "Program Files")}, "")
	if got := l.RunDir(); got != filepath.Join(home, "openvpn", "config", "dd-runtime") {
		t.Fatalf("unexpected windows run dir: %s", got)
	}
	l.RunDirPath = filepath.Join(home, "custom-run")
	if got := l.RunDir(); got != filepath.Join(home, "custom-run") {
		t.Fatalf("unexpected custom run dir: %s", got)
	}
	l.RunDirPath = ""
	if got := l.OpenVPNBin(map[string]string{}); got != openvpnExe {
		t.Fatalf("unexpected windows bin: %s", got)
	}

	l2 := NewDefaultLauncher(home, "linux", map[string]string{"OPENVPN_BIN": "/env/openvpn"}, "")
	if got := l2.OpenVPNBin(map[string]string{}); got != "/env/openvpn" {
		t.Fatalf("unexpected env bin: %s", got)
	}
	l3 := NewDefaultLauncher(home, "linux", map[string]string{}, "")
	if got := l3.OpenVPNBin(map[string]string{"OPENVPN_BIN": "/file/openvpn"}); got != "/file/openvpn" {
		t.Fatalf("unexpected file bin: %s", got)
	}
	l7 := NewDefaultLauncher(home, "linux", map[string]string{}, "")
	if got := l7.OpenVPNBin(map[string]string{}); got != "openvpn" {
		t.Fatalf("expected linux fallback bin, got %s", got)
	}
	l5 := NewDefaultLauncher(home, "windows", map[string]string{}, "")
	if got := l5.OpenVPNBin(map[string]string{"OPENVPN_BIN": `C:\vpn\openvpn.exe`}); got != `C:\vpn\openvpn.exe` {
		t.Fatalf("unexpected windows env bin: %s", got)
	}
	l6 := NewDefaultLauncher(home, "windows", map[string]string{}, "")
	if got := l6.OpenVPNBin(map[string]string{}); got != "openvpn.exe" {
		t.Fatalf("expected windows fallback bin, got %s", got)
	}
	if l3.windowsCandidate("ProgramFiles", "OpenVPN") != "" {
		t.Fatal("expected empty windows candidate")
	}
	l4 := NewDefaultLauncher(home, "windows", map[string]string{"ProgramFiles": filepath.Join(home, "Program Files")}, openvpnExe)
	if got := l4.OpenVPNBin(map[string]string{}); got != openvpnExe {
		t.Fatalf("expected explicit openvpn path, got %s", got)
	}
}

func TestDefaultLauncherStartStopUnix(t *testing.T) {
	home := t.TempDir()
	l := NewDefaultLauncher(home, "linux", map[string]string{}, "/fake/openvpn")
	state := map[int]bool{}
	nextPID := 1000
	l.RunCommand = func(name string, args ...string) (int, error) {
		nextPID++
		pid := nextPID
		if err := os.MkdirAll(l.RunDir(), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(l.PIDFile(), []byte("1001\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(l.LogFile(), []byte("Initialization Sequence Completed\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		state[1001] = true
		if len(args) < 9 ||
			args[0] != "--daemon" ||
			args[5] != "--config" ||
			args[6] != "/tmp/config.ovpn" ||
			args[7] != "--auth-user-pass" ||
			args[8] != "/tmp/auth.txt" ||
			args[9] != "--auth-retry" ||
			args[10] != "nointeract" {
			t.Fatalf("unexpected unix args: %#v", args)
		}
		_ = pid
		return 0, nil
	}
	l.ProbePID = func(pid int) bool { return state[pid] }
	l.TerminatePID = func(pid int) error {
		delete(state, pid)
		return nil
	}
	l.Sleep = func(time.Duration) {}
	pid, err := l.Start(map[string]string{}, "/tmp/config.ovpn", "/tmp/auth.txt")
	if err != nil || pid != 1001 {
		t.Fatalf("unexpected start result: %d %v", pid, err)
	}
	if !l.IsConnected() {
		t.Fatal("expected connected")
	}
	if stop := l.Stop(); stop != 1001 {
		t.Fatalf("unexpected stop pid: %d", stop)
	}
	if l.IsConnected() {
		t.Fatal("expected disconnected")
	}
}

func TestDefaultLauncherStartFailuresAndWindows(t *testing.T) {
	home := t.TempDir()
	l := NewDefaultLauncher(home, "linux", map[string]string{}, "/fake/openvpn")
	l.RunCommand = func(name string, args ...string) (int, error) { return 1, nil }
	l.Sleep = func(time.Duration) {}
	if _, err := l.Start(map[string]string{}, "/tmp/config.ovpn", "/tmp/auth.txt"); err == nil {
		t.Fatal("expected nonzero exit failure")
	}
	l.RunCommand = func(name string, args ...string) (int, error) { return 0, errors.New("boom") }
	if _, err := l.Start(map[string]string{}, "/tmp/config.ovpn", "/tmp/auth.txt"); err == nil {
		t.Fatal("expected command error")
	}
	l.RunCommand = func(name string, args ...string) (int, error) {
		if err := os.MkdirAll(l.RunDir(), 0o755); err != nil {
			t.Fatal(err)
		}
		return 0, nil
	}
	l.ProbePID = func(pid int) bool { return false }
	if _, err := l.Start(map[string]string{}, "/tmp/config.ovpn", "/tmp/auth.txt"); err == nil || !strings.Contains(err.Error(), "OpenVPN did not create a pid file") {
		t.Fatalf("expected missing pid file error, got %v", err)
	}
	l.RunCommand = func(name string, args ...string) (int, error) {
		if err := os.MkdirAll(l.RunDir(), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(l.PIDFile(), []byte("1234\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return 0, nil
	}
	if _, err := l.Start(map[string]string{}, "/tmp/config.ovpn", "/tmp/auth.txt"); err == nil || !strings.Contains(err.Error(), "OpenVPN process is not running after connect attempt") {
		t.Fatalf("expected dead pid error, got %v", err)
	}

	w := NewDefaultLauncher(home, "windows", map[string]string{}, "")
	pids := map[int]bool{}
	w.StartProcess = func(name string, args ...string) (int, error) {
		if len(args) < 10 ||
			args[4] != "--config" ||
			args[5] != `C:\vpn\config.ovpn` ||
			args[6] != "--auth-user-pass" ||
			args[7] != `C:\vpn\auth.txt` {
			t.Fatalf("unexpected windows args: %#v", args)
		}
		pids[4321] = true
		return 4321, nil
	}
	w.CaptureOutput = func(name string, args ...string) (string, error) {
		if name == "tasklist" {
			return "openvpn.exe 4321\n", nil
		}
		if name == "taskkill" {
			delete(pids, 4321)
			return "killed", nil
		}
		return "", nil
	}
	w.Sleep = func(time.Duration) {}
	pid, err := w.Start(map[string]string{}, `C:\vpn\config.ovpn`, `C:\vpn\auth.txt`)
	if err != nil || pid != 4321 {
		t.Fatalf("unexpected windows start: %d %v", pid, err)
	}
	if !w.PIDAlive(4321) {
		t.Fatal("expected pid alive")
	}
	if stop := w.Stop(); stop != 4321 {
		t.Fatalf("unexpected windows stop pid: %d", stop)
	}

	w2 := NewDefaultLauncher(home, "windows", map[string]string{}, "")
	w2.StartProcess = func(name string, args ...string) (int, error) { return 0, nil }
	w2.Sleep = func(time.Duration) {}
	if _, err := w2.Start(map[string]string{}, `C:\vpn\config.ovpn`, `C:\vpn\auth.txt`); err == nil {
		t.Fatal("expected windows spawn failure")
	}
	w3 := NewDefaultLauncher(home, "windows", map[string]string{}, "")
	w3.StartProcess = func(name string, args ...string) (int, error) { return 99, errors.New("boom") }
	if _, err := w3.Start(map[string]string{}, `C:\vpn\config.ovpn`, `C:\vpn\auth.txt`); err == nil {
		t.Fatal("expected windows start error")
	}
	wErr := NewDefaultLauncher(home, "windows", map[string]string{}, "")
	if err := os.MkdirAll(wErr.RunDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(wErr.PIDFile(), 0o755); err != nil {
		t.Fatal(err)
	}
	wErr.StartProcess = func(name string, args ...string) (int, error) { return 5555, nil }
	wErr.CaptureOutput = func(name string, args ...string) (string, error) {
		if name == "tasklist" {
			return "openvpn.exe 5555\n", nil
		}
		return "", nil
	}
	wErr.Sleep = func(time.Duration) {}
	if _, err := wErr.Start(map[string]string{}, `C:\vpn\config.ovpn`, `C:\vpn\auth.txt`); err == nil {
		t.Fatal("expected windows pid write error")
	}
	w4Home := t.TempDir()
	w4 := NewDefaultLauncher(w4Home, "windows", map[string]string{}, "")
	if err := os.MkdirAll(w4.RunDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w4.PIDFile(), []byte("1234\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pids[1234] = false
	w4.CaptureOutput = func(name string, args ...string) (string, error) {
		if name == "tasklist" {
			return "", errors.New("missing")
		}
		return "", nil
	}
	w4.StartProcess = func(name string, args ...string) (int, error) { return 2222, nil }
	w4.Sleep = func(time.Duration) {}
	if _, err := w4.Start(map[string]string{}, `C:\vpn\config.ovpn`, `C:\vpn\auth.txt`); err == nil || (!strings.Contains(err.Error(), "OpenVPN process is not running after connect attempt") && !strings.Contains(err.Error(), "OpenVPN did not create a pid file")) {
		t.Fatalf("expected windows dead pid error, got %v", err)
	}

	w5Home := t.TempDir()
	w5 := NewDefaultLauncher(w5Home, "windows", map[string]string{}, "")
	if err := os.MkdirAll(w5.RunDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w5.PIDFile(), []byte("7777\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	w5.StartProcess = func(name string, args ...string) (int, error) { return 6666, nil }
	w5.CaptureOutput = func(name string, args ...string) (string, error) {
		if name == "tasklist" {
			return "openvpn.exe 7777\n", nil
		}
		return "", nil
	}
	w5.Sleep = func(time.Duration) {}
	pid, err = w5.Start(map[string]string{}, `C:\vpn\config.ovpn`, `C:\vpn\auth.txt`)
	if err != nil || pid != 7777 {
		t.Fatalf("expected pidfile pid success, got %d %v", pid, err)
	}
}

func TestDefaultLauncherCaptureAndPidHelpers(t *testing.T) {
	home := t.TempDir()
	l := NewDefaultLauncher(home, "linux", map[string]string{}, "")
	if rc, err := l.defaultRunCommand("sh", "-c", "exit 0"); err != nil || rc != 0 {
		t.Fatalf("unexpected default run result: %d %v", rc, err)
	}
	if out, err := l.defaultCaptureOutput("sh", "-c", "printf helper"); err != nil || out != "helper" {
		t.Fatalf("unexpected capture: %q %v", out, err)
	}
	pid, err := l.defaultStartProcess("sh", "-c", "sleep 1")
	if err != nil || pid == 0 {
		t.Fatalf("unexpected start process: %d %v", pid, err)
	}
	if !l.defaultProbePID(os.Getpid()) {
		t.Fatal("expected current pid probe success")
	}
	if err := l.defaultTerminatePID(pid); err != nil {
		t.Fatalf("unexpected terminate error: %v", err)
	}
	if l.CurrentPID() != 0 {
		t.Fatal("expected no pid file by default")
	}
	if rc, err := l.defaultRunCommand("/definitely/missing-command"); err == nil || rc != 1 {
		t.Fatalf("expected missing command error, got %d %v", rc, err)
	}
	if rc, err := l.defaultRunCommand("sh", "-c", "exit 7"); err != nil || rc != 7 {
		t.Fatalf("expected shell exit code 7, got %d %v", rc, err)
	}
	if _, err := l.defaultStartProcess("/definitely/missing-command"); err == nil {
		t.Fatal("expected start process error")
	}
	if l.defaultProbePID(-1) {
		t.Fatal("expected invalid pid probe failure")
	}
	if err := l.defaultTerminatePID(-1); err == nil {
		t.Fatal("expected invalid pid terminate error")
	}
	badPIDFile := NewDefaultLauncher(home, "linux", map[string]string{}, "")
	if err := os.MkdirAll(badPIDFile.RunDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badPIDFile.PIDFile(), []byte("not-a-number"), 0o600); err != nil {
		t.Fatal(err)
	}
	if badPIDFile.CurrentPID() != 0 {
		t.Fatal("expected bad pid parse to return zero")
	}
	if err := runSimple("sh", "-c", "exit 0"); err != nil {
		t.Fatalf("expected runSimple success, got %v", err)
	}
	if err := defaultSystem("sh", "-c", "exit 0"); err != nil {
		t.Fatalf("expected defaultSystem success, got %v", err)
	}
	if err := runSimple("/definitely/missing-command"); err == nil {
		t.Fatal("expected runSimple error")
	}
}

func TestWithLogContext(t *testing.T) {
	home := t.TempDir()
	l := NewDefaultLauncher(home, "linux", map[string]string{}, "")
	if err := os.MkdirAll(l.RunDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	msg := l.withLogContext("base failure").Error()
	if !strings.Contains(msg, "base failure") || !strings.Contains(msg, l.LogFile()) {
		t.Fatalf("unexpected empty-log context: %s", msg)
	}
	if err := os.WriteFile(l.LogFile(), []byte("one\ntwo\nthree\nfour\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	msg = l.withLogContext("base failure").Error()
	if !strings.Contains(msg, "two | three | four") {
		t.Fatalf("unexpected tailed log context: %s", msg)
	}
}

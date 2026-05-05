package mirror

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

type DefaultLauncher struct {
	Home          string
	OSType        string
	ProcessEnv    map[string]string
	RunDirPath    string
	OpenVPNPath   string
	RunCommand    func(name string, args ...string) (int, error)
	StartProcess  func(name string, args ...string) (int, error)
	CaptureOutput func(name string, args ...string) (string, error)
	ProbePID      func(pid int) bool
	TerminatePID  func(pid int) error
	Sleep         func(time.Duration)
}

func NewDefaultLauncher(home, osType string, processEnv map[string]string, openvpnPath string) *DefaultLauncher {
	l := &DefaultLauncher{
		Home:        home,
		OSType:      osType,
		ProcessEnv:  processEnv,
		RunDirPath:  "",
		OpenVPNPath: openvpnPath,
	}
	l.RunCommand = l.defaultRunCommand
	l.StartProcess = l.defaultStartProcess
	l.CaptureOutput = l.defaultCaptureOutput
	l.ProbePID = l.defaultProbePID
	l.TerminatePID = l.defaultTerminatePID
	l.Sleep = time.Sleep
	return l
}

func (l *DefaultLauncher) RunDir() string {
	if l.RunDirPath != "" {
		return l.RunDirPath
	}
	if l.isWindows() {
		return filepath.Join(l.Home, "openvpn", "config", "dd-runtime")
	}
	return filepath.Join(l.Home, ".openvpn-dd")
}

func (l *DefaultLauncher) PIDFile() string { return filepath.Join(l.RunDir(), "openvpn.pid") }
func (l *DefaultLauncher) LogFile() string { return filepath.Join(l.RunDir(), "openvpn.log") }

func (l *DefaultLauncher) OpenVPNBin(env map[string]string) string {
	if l.OpenVPNPath != "" {
		return l.OpenVPNPath
	}
	if l.ProcessEnv["OPENVPN_BIN"] != "" {
		return l.ProcessEnv["OPENVPN_BIN"]
	}
	if env["OPENVPN_BIN"] != "" {
		return env["OPENVPN_BIN"]
	}
	if l.isWindows() {
		candidates := []string{
			l.windowsCandidate("ProgramFiles", "OpenVPN", "bin", "openvpn.exe"),
			l.windowsCandidate("ProgramFiles(x86)", "OpenVPN", "bin", "openvpn.exe"),
		}
		for _, path := range candidates {
			if path == "" {
				continue
			}
			if fileExists(path) {
				return path
			}
		}
		return "openvpn.exe"
	}
	return "openvpn"
}

func (l *DefaultLauncher) Start(env map[string]string, configPath, authPath string) (int, error) {
	_ = os.MkdirAll(l.RunDir(), 0o755)
	current := l.CurrentPID()
	if current != 0 && !l.PIDAlive(current) {
		_ = os.Remove(l.PIDFile())
	}
	cmd := []string{
		"--writepid", l.PIDFile(),
		"--log", l.LogFile(),
		"--auth-user-pass", authPath,
		"--auth-retry", "nointeract",
		"--config", configPath,
		"--auth-nocache",
	}
	if l.isWindows() {
		pid, err := l.StartProcess(l.OpenVPNBin(env), cmd...)
		if err != nil {
			return 0, errors.New("openvpn failed to spawn on Windows")
		}
		if pid == 0 {
			return 0, errors.New("openvpn failed to spawn on Windows")
		}
		if !fileExists(l.PIDFile()) {
			if err := os.WriteFile(l.PIDFile(), []byte(fmt.Sprintf("%d\n", pid)), 0o600); err != nil {
				return 0, err
			}
		}
	} else {
		rc, err := l.RunCommand(l.OpenVPNBin(env), append([]string{"--daemon"}, cmd...)...)
		if err != nil {
			return 0, err
		}
		if rc != 0 {
			return 0, fmt.Errorf("openvpn failed with exit code %d", rc)
		}
	}

	l.Sleep(time.Second)
	pid := l.CurrentPID()
	if pid == 0 {
		return 0, errors.New("OpenVPN did not create a pid file")
	}
	if !l.PIDAlive(pid) {
		return 0, errors.New("OpenVPN process is not running after connect attempt")
	}
	return pid, nil
}

func (l *DefaultLauncher) CurrentPID() int {
	data, err := os.ReadFile(l.PIDFile())
	if err != nil {
		return 0
	}
	var pid int
	_, err = fmt.Sscanf(string(data), "%d", &pid)
	if err != nil {
		return 0
	}
	return pid
}

func (l *DefaultLauncher) PIDAlive(pid int) bool {
	if pid == 0 {
		return false
	}
	if l.isWindows() {
		out, err := l.CaptureOutput("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid))
		return err == nil && bytes.Contains([]byte(out), []byte(fmt.Sprintf("%d", pid)))
	}
	return l.ProbePID(pid)
}

func (l *DefaultLauncher) IsConnected() bool {
	pid := l.CurrentPID()
	return l.PIDAlive(pid)
}

func (l *DefaultLauncher) Stop() int {
	pid := l.CurrentPID()
	if pid != 0 && l.PIDAlive(pid) {
		if l.isWindows() {
			_, _ = l.CaptureOutput("taskkill", "/PID", fmt.Sprintf("%d", pid), "/T", "/F")
		} else {
			_ = l.TerminatePID(pid)
		}
		for i := 0; i < 5; i++ {
			if !l.PIDAlive(pid) {
				break
			}
			l.Sleep(time.Second)
		}
	}
	_ = os.Remove(l.PIDFile())
	return pid
}

func (l *DefaultLauncher) isWindows() bool { return l.OSType == "windows" }

func (l *DefaultLauncher) windowsCandidate(base string, parts ...string) string {
	root := l.ProcessEnv[base]
	if root == "" {
		return ""
	}
	all := append([]string{root}, parts...)
	return filepath.Join(all...)
}

func (l *DefaultLauncher) defaultRunCommand(name string, args ...string) (int, error) {
	cmd := exec.Command(name, args...)
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 1, err
}

func (l *DefaultLauncher) defaultStartProcess(name string, args ...string) (int, error) {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

func (l *DefaultLauncher) defaultCaptureOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (l *DefaultLauncher) defaultProbePID(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, _ := os.FindProcess(pid)
	return process.Signal(syscall.Signal(0)) == nil
}

func (l *DefaultLauncher) defaultTerminatePID(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid")
	}
	process, _ := os.FindProcess(pid)
	return process.Signal(syscall.SIGTERM)
}

func runSimple(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Run()
}

var _ = runtime.GOOS

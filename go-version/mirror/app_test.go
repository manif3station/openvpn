package mirror

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

type failWriter struct{}

func (failWriter) Write(p []byte) (int, error) { return 0, errors.New("write failed") }

type failReader struct{}

func (failReader) Read(p []byte) (int, error) { return 0, errors.New("read failed") }

type fakeLauncher struct {
	startedConfig string
	startedAuth   string
	pid           int
	connected     bool
	stopPID       int
	openvpnBin    string
}

func (f *fakeLauncher) OpenVPNBin(env map[string]string) string { return f.openvpnBin }
func (f *fakeLauncher) Start(env map[string]string, configPath, authPath string) (int, error) {
	f.startedConfig = configPath
	f.startedAuth = authPath
	f.connected = true
	if f.pid == 0 {
		f.pid = 4321
	}
	return f.pid, nil
}
func (f *fakeLauncher) CurrentPID() int { return f.pid }
func (f *fakeLauncher) PIDAlive(pid int) bool {
	return f.connected && pid == f.pid && pid != 0
}
func (f *fakeLauncher) IsConnected() bool { return f.connected }
func (f *fakeLauncher) Stop() int {
	f.connected = false
	f.stopPID = f.pid
	return f.pid
}

func newTestApp(t *testing.T, osType string) (*App, *fakeLauncher, string) {
	t.Helper()
	home := t.TempDir()
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	launcher := &fakeLauncher{openvpnBin: "openvpn"}
	app := &App{
		Home:        home,
		OSType:      osType,
		ProcessEnv:  map[string]string{},
		Now:         func() time.Time { return time.Unix(59, 0) },
		Sleep:       func(time.Duration) {},
		System:      func(name string, args ...string) error { return nil },
		Interactive: false,
		Stdin:       in,
		Stdout:      out,
		Stderr:      errOut,
		Launcher:    launcher,
	}
	return app, launcher, home
}

func TestDetectCommand(t *testing.T) {
	cmd, args, err := detectCommand("setup", []string{"-u", "a"})
	if err != nil || cmd != "setup" || len(args) != 2 {
		t.Fatalf("unexpected detect result: %v %v %v", cmd, args, err)
	}
	cmd, args, err = detectCommand("openvpn-go", []string{"connect", "--auto"})
	if err != nil || cmd != "connect" || len(args) != 1 {
		t.Fatalf("unexpected subcommand detect result: %v %v %v", cmd, args, err)
	}
	if _, _, err = detectCommand("openvpn-go", nil); err == nil {
		t.Fatal("expected usage error")
	}
	if _, _, err = detectCommand("openvpn-go", []string{"bogus"}); err == nil {
		t.Fatal("expected unsupported command error")
	}
}

func TestRunErrorsAndJSON(t *testing.T) {
	stderr := &bytes.Buffer{}
	code := Run("openvpn-go", []string{"bogus"}, bytes.NewBuffer(nil), io.Discard, stderr, nil)
	if code != 2 || stderr.Len() == 0 {
		t.Fatalf("expected error exit, got %d", code)
	}

	app, _, home := newTestApp(t, "linux")
	if err := os.MkdirAll(filepath.Join(home, "openvpn", "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "openvpn", "config", "client.ovpn"), []byte("client\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout := &bytes.Buffer{}
	app.Stdout = stdout
	code = Run("setup", []string{"-u", "alice", "-p", "secret", "-2fa", "654321"}, bytes.NewBuffer(nil), stdout, io.Discard, app)
	if code != 0 {
		t.Fatalf("expected success exit, got %d", code)
	}
	var result Result
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &result); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if result.Mode != "setup" || result.Username != "alice" {
		t.Fatalf("unexpected setup result: %+v", result)
	}

	connectOut := &bytes.Buffer{}
	connectErr := &bytes.Buffer{}
	connectApp, launcher, connectHome := newTestApp(t, "linux")
	connectApp.Stdout = connectOut
	connectApp.Stderr = connectErr
	if err := os.MkdirAll(filepath.Join(connectHome, "openvpn", "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(connectHome, "openvpn", "config", "client.ovpn"), []byte("client\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := connectApp.WriteEnvFile(map[string]string{"USERNAME": "alice", "PASSWORD": "secret"}); err != nil {
		t.Fatal(err)
	}
	code = Run("openvpn-go", []string{"connect", "--auto"}, bytes.NewBuffer(nil), connectOut, connectErr, connectApp)
	if code != 0 {
		t.Fatalf("expected connect success exit, got %d stderr=%q", code, connectErr.String())
	}
	launcher.connected = false
	connectOut.Reset()
	connectApp.Launcher = launcherError{msg: "boom"}
	code = Run("openvpn-go", []string{"connect"}, bytes.NewBuffer(nil), connectOut, connectErr, connectApp)
	if code != 1 {
		t.Fatalf("expected connect failure exit, got %d", code)
	}
	code = Run("openvpn-go", []string{"setup", "--bogus"}, bytes.NewBuffer(nil), io.Discard, connectErr, connectApp)
	if code != 2 {
		t.Fatalf("expected setup parse error exit, got %d", code)
	}

	discOut := &bytes.Buffer{}
	discApp, discLauncher, _ := newTestApp(t, "windows")
	discApp.Stdout = discOut
	discLauncher.connected = true
	discLauncher.pid = 501
	if code = Run("disconnect", nil, bytes.NewBuffer(nil), discOut, io.Discard, discApp); code != 0 {
		t.Fatalf("expected disconnect success exit, got %d", code)
	}
	nrOut := &bytes.Buffer{}
	nrApp, _, _ := newTestApp(t, "linux")
	nrApp.Stdout = nrOut
	if code = Run("noreconnect", nil, bytes.NewBuffer(nil), nrOut, io.Discard, nrApp); code != 0 {
		t.Fatalf("expected noreconnect success exit, got %d", code)
	}

}

func TestSetupCanonicalAndLegacyEnv(t *testing.T) {
	app, _, home := newTestApp(t, "linux")
	app.Stdin = bytes.NewBufferString("alice\nsecret-pass\nTOTPSECRET\n")
	result, err := app.ExecuteSetup(nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Username != "alice" || result.TwoFactor != "totp" {
		t.Fatalf("unexpected result: %+v", result)
	}
	content, err := os.ReadFile(filepath.Join(home, ".openvpn.env"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, want := range []string{"USERNAME=alice", "PASSWORD=secret-pass", "MFA=TOTPSECRET"} {
		if !bytes.Contains(content, []byte(want)) {
			t.Fatalf("missing %s in env file: %s", want, text)
		}
	}

	if err := os.WriteFile(filepath.Join(home, ".openvpn.env"), []byte("OPENVPN_USERNAME=legacy\nOPENVPN_PASSWORD=pass\nOPENVPN_2FA=654321\nOPENVPN_CONFIG=~/openvpn/config/work.ovpn\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env, err := app.ReadEnvFile()
	if err != nil {
		t.Fatal(err)
	}
	if env["USERNAME"] != "legacy" || env["PASSWORD"] != "pass" || env["MFA"] != "654321" || env["CONFIG"] != "~/openvpn/config/work.ovpn" {
		t.Fatalf("legacy env normalization failed: %#v", env)
	}

	if err := os.WriteFile(filepath.Join(home, ".openvpn.env"), []byte("BROKEN"), 0o600); err != nil {
		t.Fatal(err)
	}
	env, err = app.ReadEnvFile()
	if err != nil {
		t.Fatal(err)
	}
	if len(env) != 0 {
		t.Fatalf("expected broken line to be ignored, got %#v", env)
	}
}

func TestSetupConfigAndMissingPassword(t *testing.T) {
	app, _, _ := newTestApp(t, "linux")
	result, err := app.ExecuteSetup([]string{"-u", "bob", "-p", "secret", "-2fa", "123456", "-c", "~/openvpn/config/work.ovpn"})
	if err != nil {
		t.Fatal(err)
	}
	if result.ConfigCandidate != "~/openvpn/config/work.ovpn" || result.TwoFactor != "static" {
		t.Fatalf("unexpected setup config result: %+v", result)
	}
	app, _, _ = newTestApp(t, "linux")
	app.Stdin = bytes.NewBufferString("only-user\n\n")
	_, err = app.ExecuteSetup(nil)
	if err == nil || err.Error() != "OpenVPN password is required" {
		t.Fatalf("expected missing password error, got %v", err)
	}

	app, _, _ = newTestApp(t, "linux")
	app.Stdin = bytes.NewBufferString("\nsecret\n")
	_, err = app.ExecuteSetup(nil)
	if err == nil || err.Error() != "OpenVPN username is required" {
		t.Fatalf("expected missing username error, got %v", err)
	}
}

func TestExecuteSetupErrorBranches(t *testing.T) {
	app, _, _ := newTestApp(t, "linux")
	if err := os.MkdirAll(app.EnvFile(), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ExecuteSetup([]string{"-u", "alice", "-p", "secret"}); err == nil {
		t.Fatal("expected setup env read error")
	}

	app, _, home := newTestApp(t, "linux")
	app.Stdout = failWriter{}
	if err := os.MkdirAll(filepath.Join(home, "openvpn", "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ExecuteSetup(nil); err == nil {
		t.Fatal("expected username prompt write error")
	}

	app, _, _ = newTestApp(t, "linux")
	app.Interactive = true
	app.Stdin = failReader{}
	if _, err := app.ExecuteSetup([]string{"-u", "alice"}); err == nil {
		t.Fatal("expected password prompt read error")
	}

	app, _, _ = newTestApp(t, "linux")
	app.Interactive = true
	app.Stdin = failReader{}
	if _, err := app.ExecuteSetup([]string{"-u", "alice", "-p", "secret"}); err == nil {
		t.Fatal("expected mfa prompt read error")
	}

	app, _, _ = newTestApp(t, "linux")
	app.PersistEnv = func(map[string]string) error { return errors.New("env write failed") }
	if _, err := app.ExecuteSetup([]string{"-u", "alice", "-p", "secret"}); err == nil {
		t.Fatal("expected setup env write error")
	}

	app, _, _ = newTestApp(t, "linux")
	app.PersistState = func(State) error { return errors.New("state write failed") }
	if _, err := app.ExecuteSetup([]string{"-u", "alice", "-p", "secret"}); err == nil {
		t.Fatal("expected setup state write error")
	}

	app, _, _ = newTestApp(t, "linux")
	if err := app.WriteEnvFile(map[string]string{"EXTRA": "keep", "MFA": "old", "CONFIG": "~/old.ovpn"}); err != nil {
		t.Fatal(err)
	}
	result, err := app.ExecuteSetup([]string{"-u", "alice", "-p", "secret"})
	if err != nil {
		t.Fatal(err)
	}
	env, err := app.ReadEnvFile()
	if err != nil {
		t.Fatal(err)
	}
	if env["EXTRA"] != "keep" || env["MFA"] != "old" || result.ConfigCandidate != "~/old.ovpn" {
		t.Fatalf("expected existing values to persist, got env=%#v result=%+v", env, result)
	}

	app, _, _ = newTestApp(t, "linux")
	app.Stdin = bytes.NewBufferString("alice\nsecret\n\n")
	result, err = app.ExecuteSetup(nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.TwoFactor != "disabled" {
		t.Fatalf("expected disabled mfa, got %+v", result)
	}
}

func TestConnectFlowAndLegacySupport(t *testing.T) {
	app, launcher, home := newTestApp(t, "linux")
	if err := os.MkdirAll(filepath.Join(home, "openvpn", "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(home, "openvpn", "config", "client.ovpn")
	if err := os.WriteFile(cfg, []byte("client\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := app.ExecuteConnect([]string{"--collector"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "not_setup" || app.ResultExitCode(result) != 1 {
		t.Fatalf("expected not_setup collector result, got %+v", result)
	}

	if err := app.WriteEnvFile(map[string]string{"USERNAME": "alice", "PASSWORD": "secret", "MFA": "JBSWY3DPEHPK3PXP"}); err != nil {
		t.Fatal(err)
	}
	result, err = app.ExecuteConnect([]string{"--auto"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "connected" || !result.Connected {
		t.Fatalf("unexpected connected result: %+v", result)
	}
	if result.AuthFile == "" {
		t.Fatal("expected auth file path in result")
	}
	auth, err := os.ReadFile(launcher.startedAuth)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(auth, []byte("alice\nsecret")) {
		t.Fatalf("unexpected auth file: %s", auth)
	}

	if err := app.WriteEnvFile(map[string]string{
		"OPENVPN_USERNAME": "legacy-alice",
		"OPENVPN_PASSWORD": "legacy-secret",
		"OPENVPN_2FA":      "123456",
		"OPENVPN_CONFIG":   "~/openvpn/config/client.ovpn",
	}); err != nil {
		t.Fatal(err)
	}
	launcher.connected = false
	result, err = app.ExecuteConnect([]string{"--auto"})
	if err != nil {
		t.Fatal(err)
	}
	auth, err = os.ReadFile(launcher.startedAuth)
	if err != nil {
		t.Fatal(err)
	}
	if string(auth) != "legacy-alice\nlegacy-secret123456\n" {
		t.Fatalf("legacy auth file mismatch: %q", string(auth))
	}
	if result.Status != "connected" {
		t.Fatalf("unexpected legacy connect result: %+v", result)
	}

	launcher.connected = true
	result, err = app.ExecuteConnect(nil)
	if err != nil || result.Status != "connected" || result.AutoReconnect {
		t.Fatalf("unexpected one-off connected result: %+v %v", result, err)
	}
}

func TestCollectorConnectedRetryDisableAndManualFailure(t *testing.T) {
	app, launcher, home := newTestApp(t, "linux")
	if err := os.MkdirAll(filepath.Join(home, "openvpn", "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "openvpn", "config", "client.ovpn"), []byte("client\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := app.WriteEnvFile(map[string]string{"USERNAME": "alice", "PASSWORD": "secret"}); err != nil {
		t.Fatal(err)
	}
	if err := app.WriteState(State{AutoReconnect: true, RetryCount: 0, Status: "connected"}); err != nil {
		t.Fatal(err)
	}
	launcher.connected = true
	launcher.pid = 999
	result, err := app.ExecuteConnect([]string{"--collector"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "connected" || result.PID != 999 {
		t.Fatalf("unexpected collector connected result: %+v", result)
	}

	failApp, _, failHome := newTestApp(t, "linux")
	if err := os.MkdirAll(filepath.Join(failHome, "openvpn", "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(failHome, "openvpn", "config", "client.ovpn"), []byte("client\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := failApp.WriteEnvFile(map[string]string{"USERNAME": "alice", "PASSWORD": "secret"}); err != nil {
		t.Fatal(err)
	}
	failApp.Launcher = launcherError{msg: "manual failure"}
	result, err = failApp.ExecuteConnect([]string{"--collector"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "retrying" || result.RetryCount != 1 {
		t.Fatalf("unexpected retry result: %+v", result)
	}
	if err := failApp.WriteState(State{AutoReconnect: true, RetryCount: 4, Status: "retrying"}); err != nil {
		t.Fatal(err)
	}
	result, err = failApp.ExecuteConnect([]string{"--collector"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "reconnect_disabled" || result.AutoReconnect {
		t.Fatalf("unexpected disable result: %+v", result)
	}
	result, err = failApp.ExecuteConnect([]string{"--auto"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "retrying" {
		t.Fatalf("unexpected manual auto failure result: %+v", result)
	}
	result = failApp.handleConnectFailure("connect", false, map[string]string{}, State{}, "manual failure")
	if result.Status != "failed" {
		t.Fatalf("expected failed manual status, got %+v", result)
	}

	if err := failApp.WriteState(State{AutoReconnect: false, RetryCount: 3, Status: "disabled"}); err != nil {
		t.Fatal(err)
	}
	result, err = failApp.ExecuteConnect([]string{"--collector"})
	if err != nil || result.Status != "reconnect_disabled" {
		t.Fatalf("unexpected reconnect_disabled collector result: %+v %v", result, err)
	}
}

type launcherError struct{ msg string }

func (l launcherError) OpenVPNBin(env map[string]string) string { return "openvpn" }
func (l launcherError) Start(env map[string]string, configPath, authPath string) (int, error) {
	return 0, errors.New(l.msg)
}
func (l launcherError) CurrentPID() int       { return 0 }
func (l launcherError) PIDAlive(pid int) bool { return false }
func (l launcherError) IsConnected() bool     { return false }
func (l launcherError) Stop() int             { return 0 }

func TestDisconnectNoReconnectAndPrompts(t *testing.T) {
	app, launcher, home := newTestApp(t, "windows")
	if err := os.MkdirAll(filepath.Join(home, "openvpn", "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "openvpn", "config", "client.ovpn"), []byte("client\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	launcher.pid = 321
	launcher.connected = true
	if err := app.WriteState(State{AutoReconnect: true, RetryCount: 2, Status: "connected"}); err != nil {
		t.Fatal(err)
	}
	result, err := app.ExecuteNoReconnect(nil)
	if err != nil || result.Status != "reconnect_disabled" {
		t.Fatalf("unexpected noreconnect result: %+v %v", result, err)
	}
	result, err = app.ExecuteDisconnect(nil)
	if err != nil || result.Status != "disconnected" || result.StoppedPID != 321 {
		t.Fatalf("unexpected disconnect result: %+v %v", result, err)
	}
	if _, err := app.ExecuteDisconnect([]string{"--bogus"}); err == nil {
		t.Fatal("expected disconnect arg error")
	}
	if _, err := app.ExecuteNoReconnect([]string{"--bogus"}); err == nil {
		t.Fatal("expected noreconnect arg error")
	}

	hiddenOut := &bytes.Buffer{}
	app.Stdout = hiddenOut
	app.Stdin = bytes.NewBufferString("visible\n")
	app.Interactive = true
	got, err := app.promptHidden("Prompt: ")
	if err != nil || got != "visible" {
		t.Fatalf("unexpected windows prompt hidden result: %q %v", got, err)
	}
	if hiddenOut.String() != "Prompt: " {
		t.Fatalf("unexpected prompt output: %q", hiddenOut.String())
	}
}

func TestPromptHiddenUnixAndHelpers(t *testing.T) {
	app, _, _ := newTestApp(t, "linux")
	out := &bytes.Buffer{}
	app.Stdout = out
	app.Stdin = bytes.NewBufferString("secret\n")
	app.Interactive = true
	var calls [][]string
	app.System = func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}
	got, err := app.promptHidden("Hidden: ")
	if err != nil || got != "secret" {
		t.Fatalf("unexpected unix hidden prompt result: %q %v", got, err)
	}
	if len(calls) != 2 || calls[0][1] != "-echo" || calls[1][1] != "echo" {
		t.Fatalf("unexpected stty calls: %#v", calls)
	}
	if app.ExpandTilde("/tmp/plain.ovpn") != "/tmp/plain.ovpn" {
		t.Fatal("expand tilde changed plain path")
	}
	if app.LogFile() == "" {
		t.Fatal("expected log file path")
	}
	if app.twoFactorMode("") != "disabled" || app.twoFactorMode("123456") != "static" || app.twoFactorMode("JBSWY3DPEHPK3PXP") != "totp" {
		t.Fatal("unexpected twoFactorMode result")
	}
	if app.OpenVPNBin(map[string]string{}) != "openvpn" {
		t.Fatal("unexpected openvpn bin")
	}
	if app.PIDAlive(0) {
		t.Fatal("expected zero pid to be dead")
	}
	if app.ResultExitCode(Result{Connected: true}) != 0 || app.ResultExitCode(Result{Connected: false}) != 1 {
		t.Fatal("unexpected result exit code")
	}
	if choose("a", "b") != "a" || choose("", "b") != "b" {
		t.Fatal("choose failed")
	}
	if firstNonEmpty("", "x", "y") != "x" {
		t.Fatal("firstNonEmpty failed")
	}
	if !fileExists(os.Args[0]) {
		t.Fatal("expected current test binary to exist")
	}
	if len(currentEnvMap()) == 0 {
		t.Fatal("expected environment map")
	}

	app2, _, _ := newTestApp(t, "linux")
	app2.Launcher = nil
	if app2.launcher() == nil {
		t.Fatal("expected launcher fallback")
	}
}

func TestConfigResolutionAndPaths(t *testing.T) {
	app, _, home := newTestApp(t, "windows")
	app.ProcessEnv["ProgramFiles"] = filepath.Join(home, "ProgramFiles")
	cfgd := filepath.Join(home, "ProgramFiles", "OpenVPN", "config")
	if err := os.MkdirAll(cfgd, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(cfgd, "other.ovpn")
	if err := os.WriteFile(cfg, []byte("client\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := app.FindOpenVPNConfig(); got != cfg {
		t.Fatalf("unexpected windows config path: %s", got)
	}

	appMissing, _, _ := newTestApp(t, runtime.GOOS)
	if _, err := appMissing.ResolvedOpenVPNConfig(map[string]string{}); err == nil {
		t.Fatal("expected missing config error")
	}

	app2, _, home2 := newTestApp(t, runtime.GOOS)
	dir := filepath.Join(home2, "openvpn", "config")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(dir, "client.ovpn")
	if err := os.WriteFile(first, []byte("client\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := app2.FindOpenVPNConfig(); got != first {
		t.Fatalf("unexpected unix config path: %s", got)
	}
	if err := app2.WriteEnvFile(map[string]string{"USERNAME": "alice", "PASSWORD": "secret", "CONFIG": "~/openvpn/config/client.ovpn"}); err != nil {
		t.Fatal(err)
	}
	got, err := app2.ResolvedOpenVPNConfig(nil)
	if err != nil || got != first {
		t.Fatalf("unexpected resolved config: %s %v", got, err)
	}
	app2.ProcessEnv["CONFIG"] = "~/openvpn/config/client.ovpn"
	if got, err := app2.ResolvedOpenVPNConfig(map[string]string{}); err != nil || got != first {
		t.Fatalf("expected process env config, got %s %v", got, err)
	}
	app2.ProcessEnv["CONFIG"] = "~/openvpn/config/missing.ovpn"
	app2.ProcessEnv["OPENVPN_CONFIG"] = "~/openvpn/config/client.ovpn"
	if got, err := app2.ResolvedOpenVPNConfig(map[string]string{}); err != nil || got != first {
		t.Fatalf("expected legacy process env config, got %s %v", got, err)
	}

	readCfgApp, _, _ := newTestApp(t, "linux")
	if err := os.MkdirAll(readCfgApp.EnvFile(), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := readCfgApp.ResolvedOpenVPNConfig(nil); err == nil {
		t.Fatal("expected env read failure from nil config resolution")
	}
}

func TestArgumentParsersAndIOFailures(t *testing.T) {
	if _, err := parseSetupArgs([]string{"-u"}); err == nil {
		t.Fatal("expected missing username value")
	}
	if _, err := parseSetupArgs([]string{"--password"}); err == nil {
		t.Fatal("expected missing password value")
	}
	if _, err := parseSetupArgs([]string{"--token"}); err == nil {
		t.Fatal("expected missing token value")
	}
	if _, err := parseSetupArgs([]string{"--config"}); err == nil {
		t.Fatal("expected missing config value")
	}
	if _, err := parseConnectArgs([]string{"--bogus"}); err == nil {
		t.Fatal("expected unsupported connect option")
	}

	app, _, _ := newTestApp(t, "linux")
	app.Stdout = failWriter{}
	if _, err := app.promptVisible("Prompt: "); err == nil {
		t.Fatal("expected visible prompt write error")
	}

	app, _, _ = newTestApp(t, "linux")
	app.Stdin = failReader{}
	if _, err := app.promptVisible("Prompt: "); err == nil {
		t.Fatal("expected visible prompt read error")
	}

	app, _, _ = newTestApp(t, "linux")
	app.Stdout = failWriter{}
	app.Interactive = true
	if _, err := app.promptHidden("Prompt: "); err == nil {
		t.Fatal("expected hidden prompt write error")
	}

	app, _, _ = newTestApp(t, "linux")
	app.Interactive = true
	app.Stdin = failReader{}
	if _, err := app.promptHidden("Prompt: "); err == nil {
		t.Fatal("expected hidden prompt read error")
	}

	app, _, _ = newTestApp(t, "linux")
	if _, err := app.WriteAuthFile(map[string]string{"USERNAME": "alice", "PASSWORD": "secret", "MFA": "!!!!"}); err == nil {
		t.Fatal("expected bad mfa error")
	}

	app, _, _ = newTestApp(t, "linux")
	if err := os.MkdirAll(app.AuthFile(), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := app.WriteAuthFile(map[string]string{"USERNAME": "alice", "PASSWORD": "secret"}); err == nil {
		t.Fatal("expected auth file write error")
	}

	app, _, _ = newTestApp(t, "linux")
	if _, err := app.startConnection(map[string]string{"USERNAME": "alice", "PASSWORD": "secret"}); err == nil {
		t.Fatal("expected missing config start error")
	}
}

func TestStateAndEnvWriteErrorsAndNewApp(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "/tmp/profile-home")
	tmpFile, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	defer tmpFile.Close()
	app := NewApp(tmpFile, io.Discard, io.Discard)
	if app.Home != "/tmp/profile-home" {
		t.Fatalf("expected USERPROFILE fallback, got %s", app.Home)
	}
	if app.Launcher == nil {
		t.Fatal("expected default launcher")
	}

	readEnvApp, _, _ := newTestApp(t, "linux")
	if err := os.MkdirAll(readEnvApp.EnvFile(), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := readEnvApp.ReadEnvFile(); err == nil {
		t.Fatal("expected env read error")
	}

	writeEnvApp, _, _ := newTestApp(t, "linux")
	if err := os.MkdirAll(writeEnvApp.EnvFile(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeEnvApp.WriteEnvFile(map[string]string{"USERNAME": "alice"}); err == nil {
		t.Fatal("expected env write error")
	}

	readStateApp, _, _ := newTestApp(t, "linux")
	if err := os.MkdirAll(readStateApp.StateFile(), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := readStateApp.ReadState(); err == nil {
		t.Fatal("expected state read error")
	}

	stateApp, _, _ := newTestApp(t, "linux")
	_ = stateApp.ensureRuntimeDir()
	if err := os.WriteFile(stateApp.StateFile(), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	if state, err := stateApp.ReadState(); err != nil || state.Status != "new" {
		t.Fatalf("expected empty state fallback, got %+v %v", state, err)
	}
	if err := os.WriteFile(stateApp.StateFile(), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := stateApp.ReadState(); err == nil {
		t.Fatal("expected bad state json error")
	}

	writeStateApp, _, _ := newTestApp(t, "linux")
	if err := os.MkdirAll(writeStateApp.StateFile(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeStateApp.WriteState(State{Status: "x"}); err == nil {
		t.Fatal("expected state write error")
	}

	connectReadEnvApp, _, _ := newTestApp(t, "linux")
	if err := os.MkdirAll(connectReadEnvApp.EnvFile(), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := connectReadEnvApp.ExecuteConnect(nil); err == nil {
		t.Fatal("expected connect env read error")
	}

	connectReadStateApp, _, home := newTestApp(t, "linux")
	if err := os.MkdirAll(filepath.Join(home, "openvpn", "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "openvpn", "config", "client.ovpn"), []byte("client\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := connectReadStateApp.WriteEnvFile(map[string]string{"USERNAME": "alice", "PASSWORD": "secret"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(connectReadStateApp.StateFile(), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := connectReadStateApp.ExecuteConnect(nil); err == nil {
		t.Fatal("expected connect state read error")
	}

	discWriteStateApp, launcher, _ := newTestApp(t, "linux")
	launcher.connected = true
	launcher.pid = 44
	discWriteStateApp.PersistState = func(State) error { return errors.New("state write failed") }
	if _, err := discWriteStateApp.ExecuteDisconnect(nil); err == nil {
		t.Fatal("expected disconnect state write error")
	}

	nrReadStateApp, _, _ := newTestApp(t, "linux")
	if err := os.MkdirAll(nrReadStateApp.StateFile(), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := nrReadStateApp.ExecuteNoReconnect(nil); err == nil {
		t.Fatal("expected noreconnect state read error")
	}

	nrWriteStateApp, _, _ := newTestApp(t, "linux")
	if err := nrWriteStateApp.WriteState(State{AutoReconnect: true, RetryCount: 1, Status: "connected"}); err != nil {
		t.Fatal(err)
	}
	nrWriteStateApp.PersistState = func(State) error { return errors.New("state write failed") }
	if _, err := nrWriteStateApp.ExecuteNoReconnect(nil); err == nil {
		t.Fatal("expected noreconnect state write error")
	}
}

func TestExecuteConnectErrorBranches(t *testing.T) {
	app, launcher, home := newTestApp(t, "linux")
	if err := os.MkdirAll(filepath.Join(home, "openvpn", "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(home, "openvpn", "config", "client.ovpn")
	if err := os.WriteFile(cfg, []byte("client\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := app.WriteEnvFile(map[string]string{"USERNAME": "alice", "PASSWORD": "secret"}); err != nil {
		t.Fatal(err)
	}
	_ = app.ensureRuntimeDir()
	if _, err := app.ExecuteConnect([]string{"--bogus"}); err == nil {
		t.Fatal("expected connect parse error")
	}

	if err := app.WriteState(State{AutoReconnect: true, RetryCount: 1, Status: "connected"}); err != nil {
		t.Fatal(err)
	}
	app.PersistState = func(State) error { return errors.New("state write failed") }
	if _, err := app.ExecuteConnect([]string{"--auto"}); err == nil {
		t.Fatal("expected connect auto state write error")
	}
	app.PersistState = nil

	if err := app.WriteState(State{AutoReconnect: true, RetryCount: 1, Status: "connected"}); err != nil {
		t.Fatal(err)
	}
	app.PersistState = func(State) error { return errors.New("state write failed") }
	if _, err := app.ExecuteConnect(nil); err == nil {
		t.Fatal("expected connect manual state write error")
	}
	app.PersistState = nil

	launcher.connected = true
	if err := os.Remove(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ExecuteConnect(nil); err == nil {
		t.Fatal("expected connected config resolution error")
	}
	if err := os.WriteFile(cfg, []byte("client\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := app.WriteState(State{AutoReconnect: false, RetryCount: 2, Status: "disabled"}); err != nil {
		t.Fatal(err)
	}
	launcher.connected = false
	if err := os.Remove(cfg); err != nil {
		t.Fatal(err)
	}
	result, err := app.ExecuteConnect([]string{"--collector"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "reconnect_disabled" || result.Config != "" {
		t.Fatalf("expected collector disabled with empty config, got %+v", result)
	}
	if err := os.WriteFile(cfg, []byte("client\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := app.WriteEnvFile(map[string]string{"USERNAME": "alice", "PASSWORD": "secret", "MFA": "!!!!"}); err != nil {
		t.Fatal(err)
	}
	launcher.connected = false
	if _, err := app.startConnection(map[string]string{"USERNAME": "alice", "PASSWORD": "secret", "MFA": "!!!!", "CONFIG": cfg}); err == nil {
		t.Fatal("expected startConnection auth error")
	}

	app2, _, home2 := newTestApp(t, "linux")
	if err := os.MkdirAll(filepath.Join(home2, "openvpn", "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg2 := filepath.Join(home2, "openvpn", "config", "client.ovpn")
	if err := os.WriteFile(cfg2, []byte("client\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := app2.WriteEnvFile(map[string]string{"USERNAME": "alice", "PASSWORD": "secret"}); err != nil {
		t.Fatal(err)
	}
	if err := app2.WriteState(State{AutoReconnect: true, RetryCount: 0, Status: "new"}); err != nil {
		t.Fatal(err)
	}
	app2.Launcher = &fakeLauncher{openvpnBin: "openvpn", pid: 90}
	app2.PersistState = func(State) error { return errors.New("state write failed") }
	if _, err := app2.ExecuteConnect([]string{"--auto"}); err == nil {
		t.Fatal("expected post-connect state write error")
	}
	app2.PersistState = nil

	appSuccess, launcherSuccess, homeSuccess := newTestApp(t, "linux")
	if err := os.MkdirAll(filepath.Join(homeSuccess, "openvpn", "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeSuccess, "openvpn", "config", "client.ovpn"), []byte("client\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := appSuccess.WriteEnvFile(map[string]string{"USERNAME": "alice", "PASSWORD": "secret"}); err != nil {
		t.Fatal(err)
	}
	if err := appSuccess.WriteState(State{AutoReconnect: true, RetryCount: 1, Status: "retrying"}); err != nil {
		t.Fatal(err)
	}
	launcherSuccess.connected = false
	result, err = appSuccess.ExecuteConnect([]string{"--collector"})
	if err != nil || result.Status != "reconnected" || !result.AutoReconnect {
		t.Fatalf("expected collector reconnect success, got %+v %v", result, err)
	}

	appPersistFail, _, homePersistFail := newTestApp(t, "linux")
	if err := os.MkdirAll(filepath.Join(homePersistFail, "openvpn", "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homePersistFail, "openvpn", "config", "client.ovpn"), []byte("client\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := appPersistFail.WriteEnvFile(map[string]string{"USERNAME": "alice", "PASSWORD": "secret"}); err != nil {
		t.Fatal(err)
	}
	if err := appPersistFail.WriteState(State{AutoReconnect: true, RetryCount: 0, Status: "retrying"}); err != nil {
		t.Fatal(err)
	}
	appPersistFail.PersistState = func(State) error { return errors.New("state write failed") }
	if _, err := appPersistFail.ExecuteConnect([]string{"--collector"}); err == nil {
		t.Fatal("expected collector post-connect state write error")
	}

	app3, launcher3, home3 := newTestApp(t, "linux")
	if err := os.MkdirAll(filepath.Join(home3, "openvpn", "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg3 := filepath.Join(home3, "openvpn", "config", "client.ovpn")
	if err := os.WriteFile(cfg3, []byte("client\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := app3.WriteEnvFile(map[string]string{"USERNAME": "alice", "PASSWORD": "secret", "CONFIG": "~/openvpn/config/client.ovpn"}); err != nil {
		t.Fatal(err)
	}
	launcher3.pid = 91
	launcher3.connected = false
	app3.Launcher = &fakeLauncher{openvpnBin: "openvpn", pid: 91}
	app3.Launcher = launcher3
	origLauncher := launcher3
	app3.Launcher = launcherErrorAfterStart{fake: origLauncher, removePath: cfg3}
	if _, err := app3.ExecuteConnect([]string{"--auto"}); err == nil {
		t.Fatal("expected post-start config resolution error")
	}
}

type launcherErrorAfterStart struct {
	fake       *fakeLauncher
	removePath string
}

func (l launcherErrorAfterStart) OpenVPNBin(env map[string]string) string {
	return l.fake.OpenVPNBin(env)
}
func (l launcherErrorAfterStart) Start(env map[string]string, configPath, authPath string) (int, error) {
	pid, err := l.fake.Start(env, configPath, authPath)
	if err == nil {
		_ = os.Remove(l.removePath)
	}
	return pid, err
}
func (l launcherErrorAfterStart) CurrentPID() int       { return l.fake.CurrentPID() }
func (l launcherErrorAfterStart) PIDAlive(pid int) bool { return l.fake.PIDAlive(pid) }
func (l launcherErrorAfterStart) IsConnected() bool     { return l.fake.IsConnected() }
func (l launcherErrorAfterStart) Stop() int             { return l.fake.Stop() }

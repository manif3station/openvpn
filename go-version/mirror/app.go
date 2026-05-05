package mirror

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type Launcher interface {
	OpenVPNBin(env map[string]string) string
	Start(env map[string]string, configPath, authPath string) (int, error)
	CurrentPID() int
	PIDAlive(pid int) bool
	IsConnected() bool
	Stop() int
}

type App struct {
	Home         string
	OSType       string
	ProcessEnv   map[string]string
	Now          func() time.Time
	Sleep        func(time.Duration)
	System       func(name string, args ...string) error
	Interactive  bool
	Stdin        io.Reader
	Stdout       io.Writer
	Stderr       io.Writer
	Launcher     Launcher
	PersistEnv   func(map[string]string) error
	PersistState func(State) error
	reader       *bufio.Reader
}

type State struct {
	AutoReconnect bool   `json:"auto_reconnect"`
	RetryCount    int    `json:"retry_count"`
	Status        string `json:"status"`
}

type Result struct {
	Mode          string `json:"mode"`
	Status        string `json:"status"`
	StatusIcon    string `json:"status_icon"`
	Connected     bool   `json:"connected"`
	AutoReconnect bool   `json:"auto_reconnect"`
	RetryCount    int    `json:"retry_count"`
	EnvFile       string `json:"env_file"`
	StateFile     string `json:"state_file"`
	PIDFile       string `json:"pid_file"`
	Config        string `json:"config"`
	Message       string `json:"message"`
	PID           int    `json:"pid"`
	AuthFile      string `json:"auth_file,omitempty"`
	StoppedPID    int    `json:"stopped_pid,omitempty"`

	Username        string `json:"username,omitempty"`
	TwoFactor       string `json:"two_factor,omitempty"`
	ConfigCandidate string `json:"config_candidate,omitempty"`
}

func NewApp(stdin io.Reader, stdout, stderr io.Writer) *App {
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}
	app := &App{
		Home:        home,
		OSType:      runtime.GOOS,
		ProcessEnv:  currentEnvMap(),
		Now:         time.Now,
		Sleep:       time.Sleep,
		System:      defaultSystem,
		Interactive: false,
		Stdin:       stdin,
		Stdout:      stdout,
		Stderr:      stderr,
	}
	if f, ok := stdin.(*os.File); ok {
		info, err := f.Stat()
		app.Interactive = err == nil && (info.Mode()&os.ModeCharDevice) != 0
	}
	app.Launcher = NewDefaultLauncher(app.Home, app.OSType, app.ProcessEnv, "")
	return app
}

func Run(arg0 string, argv []string, stdin io.Reader, stdout, stderr io.Writer, app *App) int {
	if app == nil {
		app = NewApp(stdin, stdout, stderr)
	}
	command, args, err := detectCommand(arg0, argv)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 2
	}

	var result Result
	switch command {
	case "setup":
		result, err = app.ExecuteSetup(args)
	case "connect":
		result, err = app.ExecuteConnect(args)
	case "disconnect":
		result, err = app.ExecuteDisconnect(args)
	case "noreconnect":
		result, err = app.ExecuteNoReconnect(args)
	}
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 2
	}

	payload, _ := json.Marshal(result)
	fmt.Fprintln(stdout, string(payload))
	if command == "connect" {
		return app.ResultExitCode(result)
	}
	return 0
}

func detectCommand(arg0 string, argv []string) (string, []string, error) {
	base := strings.TrimSuffix(filepath.Base(arg0), filepath.Ext(arg0))
	switch base {
	case "setup", "connect", "disconnect", "noreconnect":
		return base, argv, nil
	}
	if len(argv) == 0 {
		return "", nil, errors.New("usage: <setup|connect|disconnect|noreconnect> [args]")
	}
	switch argv[0] {
	case "setup", "connect", "disconnect", "noreconnect":
		return argv[0], argv[1:], nil
	default:
		return "", nil, fmt.Errorf("unsupported command: %s", argv[0])
	}
}

func (a *App) ExecuteSetup(argv []string) (Result, error) {
	opt, err := parseSetupArgs(argv)
	if err != nil {
		return Result{}, err
	}
	_ = a.ensureRuntimeDir()

	existing, err := a.ReadEnvFile()
	if err != nil {
		return Result{}, err
	}
	username := choose(opt.Username, existing["USERNAME"])
	password := choose(opt.Password, existing["PASSWORD"])
	mfa := choose(opt.MFA, existing["MFA"])
	config := choose(opt.Config, existing["CONFIG"])

	if username == "" {
		username, err = a.promptVisible("OpenVPN username: ")
		if err != nil {
			return Result{}, err
		}
	}
	if password == "" {
		password, err = a.promptHidden("OpenVPN password: ")
		if err != nil {
			return Result{}, err
		}
	}
	if mfa == "" && opt.MFA == "" && existing["MFA"] == "" {
		mfa, err = a.promptHidden("OpenVPN 2FA token or secret (optional): ")
		if err != nil {
			return Result{}, err
		}
	}
	if username == "" {
		return Result{}, errors.New("OpenVPN username is required")
	}
	if password == "" {
		return Result{}, errors.New("OpenVPN password is required")
	}

	merged := map[string]string{
		"USERNAME": username,
		"PASSWORD": password,
	}
	for k, v := range existing {
		if v != "" {
			merged[k] = v
		}
	}
	merged["USERNAME"] = username
	merged["PASSWORD"] = password
	if mfa != "" {
		merged["MFA"] = mfa
	} else {
		delete(merged, "MFA")
	}
	if config != "" {
		merged["CONFIG"] = config
	} else {
		delete(merged, "CONFIG")
	}
	if err := a.persistEnvFile(merged); err != nil {
		return Result{}, err
	}
	if err := a.persistState(State{AutoReconnect: true, RetryCount: 0, Status: "setup"}); err != nil {
		return Result{}, err
	}
	configCandidate := config
	if configCandidate == "" {
		configCandidate = a.FindOpenVPNConfig()
	}
	return Result{
		Mode:            "setup",
		EnvFile:         a.EnvFile(),
		Username:        username,
		TwoFactor:       a.twoFactorMode(mfa),
		AutoReconnect:   true,
		ConfigCandidate: configCandidate,
	}, nil
}

func (a *App) ExecuteConnect(argv []string) (Result, error) {
	opt, err := parseConnectArgs(argv)
	if err != nil {
		return Result{}, err
	}
	_ = a.ensureRuntimeDir()
	env, err := a.ReadEnvFile()
	if err != nil {
		return Result{}, err
	}
	if !a.IsSetupComplete(env) {
		mode := "connect"
		if opt.Collector {
			mode = "collector"
		}
		return a.statusResult(mode, "not_setup", "?", false, false, 0, "", "Run openvpn setup first", 0, "", 0), nil
	}

	state, err := a.ReadState()
	if err != nil {
		return Result{}, err
	}
	if opt.Auto {
		state.AutoReconnect = true
		state.RetryCount = 0
		if err := a.persistState(state); err != nil {
			return Result{}, err
		}
	} else if !opt.Collector {
		state.AutoReconnect = false
		state.RetryCount = 0
		if err := a.persistState(state); err != nil {
			return Result{}, err
		}
	}

	if a.IsConnected() {
		config, err := a.ResolvedOpenVPNConfig(env)
		if err != nil {
			return Result{}, err
		}
		mode := "connect"
		if opt.Collector {
			mode = "collector"
		}
		latest, _ := a.ReadState()
		return a.statusResult(mode, "connected", "+", true, latest.AutoReconnect, latest.RetryCount, config, "", 0, "", 0), nil
	}

	if opt.Collector && !state.AutoReconnect {
		config, err := a.ResolvedOpenVPNConfig(env)
		if err != nil {
			config = ""
		}
		return a.statusResult("collector", "reconnect_disabled", "-", false, false, state.RetryCount, config, "", 0, "", 0), nil
	}

	pid, err := a.startConnection(env)
	if err != nil {
		mode := "connect"
		if opt.Collector {
			mode = "collector"
		}
		return a.handleConnectFailure(mode, opt.Auto || opt.Collector, env, state, err.Error()), nil
	}

	autoReconnect := false
	if opt.Collector {
		autoReconnect = state.AutoReconnect
	} else {
		autoReconnect = opt.Auto
	}
	if err := a.persistState(State{AutoReconnect: autoReconnect, RetryCount: 0, Status: "connected"}); err != nil {
		return Result{}, err
	}
	config, err := a.ResolvedOpenVPNConfig(env)
	if err != nil {
		return Result{}, err
	}
	mode := "connect"
	status := "connected"
	if opt.Collector {
		mode = "collector"
		status = "reconnected"
	}
	return a.statusResult(mode, status, "+", true, autoReconnect, 0, config, "", pid, a.AuthFile(), 0), nil
}

func (a *App) ExecuteDisconnect(argv []string) (Result, error) {
	if len(argv) > 0 {
		return Result{}, fmt.Errorf("unsupported option: %s", argv[0])
	}
	_ = a.ensureRuntimeDir()
	stopped := a.StopConnection()
	if err := a.persistState(State{AutoReconnect: false, RetryCount: 0, Status: "disconnected"}); err != nil {
		return Result{}, err
	}
	return a.statusResult("disconnect", "disconnected", "x", false, false, 0, "", "", 0, "", stopped), nil
}

func (a *App) ExecuteNoReconnect(argv []string) (Result, error) {
	if len(argv) > 0 {
		return Result{}, fmt.Errorf("unsupported option: %s", argv[0])
	}
	_ = a.ensureRuntimeDir()
	state, err := a.ReadState()
	if err != nil {
		return Result{}, err
	}
	state.AutoReconnect = false
	state.Status = "reconnect_disabled"
	if err := a.persistState(state); err != nil {
		return Result{}, err
	}
	return a.statusResult("noreconnect", "reconnect_disabled", "-", a.IsConnected(), false, state.RetryCount, "", "", 0, "", 0), nil
}

type setupOptions struct {
	Username string
	Password string
	MFA      string
	Config   string
}

func parseSetupArgs(argv []string) (setupOptions, error) {
	var opt setupOptions
	for len(argv) > 0 {
		arg := argv[0]
		argv = argv[1:]
		switch arg {
		case "-u", "--username":
			if len(argv) == 0 {
				return opt, fmt.Errorf("missing value after %s", arg)
			}
			opt.Username = argv[0]
			argv = argv[1:]
		case "-p", "--password":
			if len(argv) == 0 {
				return opt, fmt.Errorf("missing value after %s", arg)
			}
			opt.Password = argv[0]
			argv = argv[1:]
		case "-2fa", "--2fa", "--token":
			if len(argv) == 0 {
				return opt, fmt.Errorf("missing value after %s", arg)
			}
			opt.MFA = argv[0]
			argv = argv[1:]
		case "-c", "--config":
			if len(argv) == 0 {
				return opt, fmt.Errorf("missing value after %s", arg)
			}
			opt.Config = argv[0]
			argv = argv[1:]
		default:
			return opt, fmt.Errorf("unsupported option: %s", arg)
		}
	}
	return opt, nil
}

type connectOptions struct {
	Auto      bool
	Collector bool
}

func parseConnectArgs(argv []string) (connectOptions, error) {
	opt := connectOptions{}
	for len(argv) > 0 {
		arg := argv[0]
		argv = argv[1:]
		switch arg {
		case "--auto":
			opt.Auto = true
		case "--collector":
			opt.Collector = true
		default:
			return opt, fmt.Errorf("unsupported option: %s", arg)
		}
	}
	return opt, nil
}

func (a *App) promptVisible(message string) (string, error) {
	if a.Stdout != nil {
		if _, err := fmt.Fprint(a.Stdout, message); err != nil {
			return "", err
		}
	}
	if a.reader == nil {
		a.reader = bufio.NewReader(a.Stdin)
	}
	line, err := a.reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func (a *App) promptHidden(message string) (string, error) {
	if !a.Interactive || a.OSType == "windows" {
		return a.promptVisible(message)
	}
	if a.Stdout != nil {
		if _, err := fmt.Fprint(a.Stdout, message); err != nil {
			return "", err
		}
	}
	_ = a.System("stty", "-echo")
	if a.reader == nil {
		a.reader = bufio.NewReader(a.Stdin)
	}
	line, err := a.reader.ReadString('\n')
	_ = a.System("stty", "echo")
	if a.Stdout != nil {
		_, _ = fmt.Fprintln(a.Stdout)
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func (a *App) ensureRuntimeDir() string {
	_ = os.MkdirAll(a.RunDir(), 0o755)
	return a.RunDir()
}

func (a *App) RunDir() string {
	if a.OSType == "windows" {
		return filepath.Join(a.Home, "openvpn", "config", "dd-runtime")
	}
	return filepath.Join(a.Home, ".openvpn-dd")
}

func (a *App) EnvFile() string   { return filepath.Join(a.Home, ".openvpn.env") }
func (a *App) StateFile() string { return filepath.Join(a.RunDir(), "state.json") }
func (a *App) AuthFile() string  { return filepath.Join(a.RunDir(), "auth.txt") }
func (a *App) PIDFile() string   { return filepath.Join(a.RunDir(), "openvpn.pid") }
func (a *App) LogFile() string   { return filepath.Join(a.RunDir(), "openvpn.log") }

func (a *App) ReadEnvFile() (map[string]string, error) {
	path := a.EnvFile()
	out := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return nil, fmt.Errorf("unable to read %s: %w", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		value := strings.Trim(parts[1], `"'`)
		out[parts[0]] = value
	}
	normalizeEnvKeys(out)
	return out, nil
}

func (a *App) WriteEnvFile(env map[string]string) error {
	keys := make([]string, 0, len(env))
	for k, v := range env {
		if v != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(env[k])
		b.WriteString("\n")
	}
	if err := os.WriteFile(a.EnvFile(), []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("unable to write %s: %w", a.EnvFile(), err)
	}
	return nil
}

func (a *App) persistEnvFile(env map[string]string) error {
	if a.PersistEnv != nil {
		return a.PersistEnv(env)
	}
	return a.WriteEnvFile(env)
}

func normalizeEnvKeys(env map[string]string) {
	if v, ok := env["OPENVPN_USERNAME"]; ok && env["USERNAME"] == "" {
		env["USERNAME"] = v
		delete(env, "OPENVPN_USERNAME")
	}
	if v, ok := env["OPENVPN_PASSWORD"]; ok && env["PASSWORD"] == "" {
		env["PASSWORD"] = v
		delete(env, "OPENVPN_PASSWORD")
	}
	if v, ok := env["OPENVPN_2FA"]; ok && env["MFA"] == "" {
		env["MFA"] = v
		delete(env, "OPENVPN_2FA")
	}
	if v, ok := env["OPENVPN_CONFIG"]; ok && env["CONFIG"] == "" {
		env["CONFIG"] = v
		delete(env, "OPENVPN_CONFIG")
	}
}

func (a *App) ReadState() (State, error) {
	path := a.StateFile()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{AutoReconnect: true, RetryCount: 0, Status: "new"}, nil
		}
		return State{}, fmt.Errorf("unable to read %s: %w", path, err)
	}
	var state State
	if len(strings.TrimSpace(string(data))) == 0 {
		return State{AutoReconnect: true, RetryCount: 0, Status: "new"}, nil
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

func (a *App) WriteState(state State) error {
	_ = a.ensureRuntimeDir()
	data, _ := json.Marshal(state)
	if err := os.WriteFile(a.StateFile(), data, 0o600); err != nil {
		return fmt.Errorf("unable to write %s: %w", a.StateFile(), err)
	}
	return nil
}

func (a *App) persistState(state State) error {
	if a.PersistState != nil {
		return a.PersistState(state)
	}
	return a.WriteState(state)
}

func (a *App) IsSetupComplete(env map[string]string) bool {
	return env["USERNAME"] != "" && env["PASSWORD"] != ""
}

func (a *App) ResolvedOpenVPNConfig(env map[string]string) (string, error) {
	if env == nil {
		var err error
		env, err = a.ReadEnvFile()
		if err != nil {
			return "", err
		}
	}
	fromEnv := firstNonEmpty(a.ProcessEnv["CONFIG"], a.ProcessEnv["OPENVPN_CONFIG"], env["CONFIG"])
	if fromEnv != "" {
		path := a.ExpandTilde(fromEnv)
		if fileExists(path) {
			return path, nil
		}
	}
	candidate := a.FindOpenVPNConfig()
	if candidate == "" {
		return "", errors.New("OpenVPN config file not found. Set CONFIG in ~/.openvpn.env or place one .ovpn file under ~/openvpn/config, ~/.openvpn, or ~/.config/openvpn")
	}
	return candidate, nil
}

func (a *App) FindOpenVPNConfig() string {
	for _, pattern := range a.defaultConfigCandidates() {
		path := pattern
		if strings.HasPrefix(path, "~/") {
			path = a.ExpandTilde(path)
		}
		if fileExists(path) {
			return path
		}
	}
	for _, dir := range a.defaultConfigSearchDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".ovpn") {
				names = append(names, entry.Name())
			}
		}
		sort.Strings(names)
		if len(names) > 0 {
			return filepath.Join(dir, names[0])
		}
	}
	return ""
}

func (a *App) startConnection(env map[string]string) (int, error) {
	config, err := a.ResolvedOpenVPNConfig(env)
	if err != nil {
		return 0, err
	}
	auth, err := a.WriteAuthFile(env)
	if err != nil {
		return 0, err
	}
	return a.launcher().Start(env, config, auth)
}

func (a *App) WriteAuthFile(env map[string]string) (string, error) {
	path := a.AuthFile()
	password := env["PASSWORD"]
	if token := env["MFA"]; token != "" {
		code, err := CurrentMFACode(token, a.Now)
		if err != nil {
			return "", err
		}
		password += code
	}
	content := env["USERNAME"] + "\n" + password + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("unable to write %s: %w", path, err)
	}
	return path, nil
}

func (a *App) OpenVPNBin(env map[string]string) string { return a.launcher().OpenVPNBin(env) }
func (a *App) CurrentPID() int                         { return a.launcher().CurrentPID() }
func (a *App) PIDAlive(pid int) bool                   { return a.launcher().PIDAlive(pid) }
func (a *App) IsConnected() bool                       { return a.launcher().IsConnected() }
func (a *App) StopConnection() int                     { return a.launcher().Stop() }

func (a *App) handleConnectFailure(mode string, auto bool, env map[string]string, state State, errorMsg string) Result {
	retry := state.RetryCount + 1
	disabled := false
	if auto {
		if retry >= 5 {
			state.AutoReconnect = false
			state.RetryCount = retry
			state.Status = "reconnect_disabled"
			disabled = true
		} else {
			state.AutoReconnect = true
			state.RetryCount = retry
			state.Status = "retrying"
		}
	} else {
		state.AutoReconnect = false
		state.RetryCount = 0
		state.Status = "failed"
	}
	_ = a.persistState(state)
	config, _ := a.ResolvedOpenVPNConfig(env)
	status := "failed"
	icon := "!"
	if disabled {
		status = "reconnect_disabled"
		icon = "-"
	} else if auto {
		status = "retrying"
	}
	return a.statusResult(mode, status, icon, false, state.AutoReconnect, state.RetryCount, config, errorMsg, 0, "", 0)
}

func (a *App) statusResult(mode, status, icon string, connected, autoReconnect bool, retry int, config, message string, pid int, authFile string, stoppedPID int) Result {
	if pid == 0 {
		pid = a.CurrentPID()
	}
	return Result{
		Mode:          mode,
		Status:        status,
		StatusIcon:    icon,
		Connected:     connected,
		AutoReconnect: autoReconnect,
		RetryCount:    retry,
		EnvFile:       a.EnvFile(),
		StateFile:     a.StateFile(),
		PIDFile:       a.PIDFile(),
		Config:        config,
		Message:       message,
		PID:           pid,
		AuthFile:      authFile,
		StoppedPID:    stoppedPID,
	}
}

func (a *App) ResultExitCode(result Result) int {
	if result.Connected {
		return 0
	}
	return 1
}

func (a *App) twoFactorMode(token string) string {
	if token == "" {
		return "disabled"
	}
	if len(token) == 6 && isDigits(token) {
		return "static"
	}
	return "totp"
}

func (a *App) ExpandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(a.Home, path[2:])
	}
	return path
}

func (a *App) defaultConfigCandidates() []string {
	if a.OSType == "windows" {
		candidates := []string{
			filepath.Join(a.Home, "openvpn", "config", "client.ovpn"),
			filepath.Join(a.Home, "openvpn", "config", "config.ovpn"),
			filepath.Join(a.Home, "OpenVPN", "config", "client.ovpn"),
			filepath.Join(a.Home, "OpenVPN", "config", "config.ovpn"),
			filepath.Join(a.Home, "config", "openvpn", "client.ovpn"),
			filepath.Join(a.Home, "config", "openvpn", "config.ovpn"),
		}
		for _, key := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
			root := a.ProcessEnv[key]
			if root != "" {
				candidates = append(candidates,
					filepath.Join(root, "OpenVPN", "config", "client.ovpn"),
					filepath.Join(root, "OpenVPN", "config", "config.ovpn"),
				)
			}
		}
		return candidates
	}
	return []string{
		"~/openvpn/config/client.ovpn",
		"~/openvpn/config/config.ovpn",
		"~/.openvpn/config.ovpn",
		"~/.openvpn/client.ovpn",
		"~/.config/openvpn/client.ovpn",
		"~/.config/openvpn/config.ovpn",
	}
}

func (a *App) defaultConfigSearchDirs() []string {
	if a.OSType == "windows" {
		dirs := []string{
			filepath.Join(a.Home, "openvpn", "config"),
			filepath.Join(a.Home, "OpenVPN", "config"),
			filepath.Join(a.Home, "config", "openvpn"),
		}
		for _, key := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
			root := a.ProcessEnv[key]
			if root != "" {
				dirs = append(dirs, filepath.Join(root, "OpenVPN", "config"))
			}
		}
		return dirs
	}
	return []string{
		a.ExpandTilde("~/openvpn/config"),
		a.ExpandTilde("~/.openvpn"),
		a.ExpandTilde("~/.config/openvpn"),
	}
}

func (a *App) launcher() Launcher {
	if a.Launcher != nil {
		return a.Launcher
	}
	a.Launcher = NewDefaultLauncher(a.Home, a.OSType, a.ProcessEnv, "")
	return a.Launcher
}

func choose(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func currentEnvMap() map[string]string {
	out := map[string]string{}
	for _, item := range os.Environ() {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 2 {
			out[parts[0]] = parts[1]
		}
	}
	return out
}

func defaultSystem(name string, args ...string) error {
	return runSimple(name, args...)
}

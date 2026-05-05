# Overview

`openvpn` is a DD operational skill for OpenVPN tunnels that need a password plus a six-digit 2FA suffix to reconnect.

The skill keeps the reconnect flow inside DD instead of pushing the user back into a manual recovery cycle every time the tunnel drops.

The skill-owned Perl modules handle the credential, TOTP, state, and process-management work inside DD. The host still needs an `openvpn` executable somewhere on `PATH`, or the user can point `OPENVPN_BIN` at it directly.

The skill now also ships a standalone Go mirror under `go-version/` for users who need the same setup/connect/disconnect/noreconnect workflow without the DD runtime.

The main behavior is:

- `dashboard openvpn.setup` records the username, password, optional 2FA secret, and optional config path in `~/.openvpn.env`
- `dashboard openvpn.setup` accepts a six-digit suffix, a raw TOTP Base32 secret, or an `otpauth://` URI for the 2FA value
- the canonical env keys are `USERNAME`, `PASSWORD`, `MFA`, and `CONFIG`, while `OPENVPN_*` keys remain readable for backward compatibility
- `dashboard openvpn.connect` performs one connection attempt and leaves reconnect disabled afterward
- `dashboard openvpn.connect --auto` performs a connection attempt and enables managed reconnect
- `dashboard openvpn.connect --collector` is the collector path that reports indicator state and retries reconnect when allowed
- `dashboard openvpn.noreconnect` disables reconnect without tearing down the current process
- `dashboard openvpn.disconnect` disconnects and disables reconnect
- the launcher starts OpenVPN with `--auth-retry nointeract` so the managed CLI path keeps using the generated auth file instead of surfacing another login dialog
- the Go mirror uses the same `~/.openvpn.env` contract and can be built into four Windows command-shaped executables from source

The collector indicator starts as `OVPN?` before setup is complete. That produces a red DD indicator state on purpose so the user sees that the skill is not ready yet.

Once setup is complete, the collector watches the managed OpenVPN process state and reconnects after disconnect when auto reconnect is enabled. After five failed reconnect attempts, it disables reconnect and returns an alert state until the user investigates and runs `dashboard openvpn.connect --auto` again.

On Windows 11 PowerShell, the launcher switches to Windows-aware process start, pid inspection, and task termination behavior, falls back to a visible prompt for hidden-password questions, and keeps its runtime helpers under `~/openvpn/config/dd-runtime`.

The Go mirror keeps that same Windows-aware layout and also supports Linux and macOS path handling. This release proved those code paths in Docker without live `macdev` or `windev` integration runs.

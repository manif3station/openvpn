# Initial Release

## What changed

- created the `openvpn` skill
- added `openvpn.setup`, `openvpn.connect`, `openvpn.disconnect`, and `openvpn.noreconnect`
- added a collector-backed reconnect path through `dashboard openvpn.connect --collector`
- stored setup values in `~/.openvpn.env`
- added OpenVPN package declarations for Debian or Ubuntu and macOS
- added TOTP and `otpauth://` secret handling for generated six-digit reconnect suffixes

## Why it changed

Some OpenVPN tunnels require a password plus a six-digit 2FA suffix during reconnect. Without a skill-owned reconnect path, those tunnels break unattended recovery and force manual intervention after each disconnect.

## Proof

- Docker tests and 100% coverage passed
- installed runtime proof passed through the real `dashboard openvpn.*` command path with a fake `openvpn` executable

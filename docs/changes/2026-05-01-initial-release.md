# Initial Release

## What changed

- created the `openvpn` skill
- added `openvpn.setup`, `openvpn.connect`, `openvpn.disconnect`, and `openvpn.noreconnect`
- added a collector-backed reconnect path through `dashboard openvpn.connect --collector`
- stored setup values in `~/.openvpn.env`
- added TOTP and `otpauth://` secret handling for generated six-digit reconnect suffixes
- moved the OpenVPN process-management lifting into skill-owned Perl modules and stopped declaring `openvpn` as an apt or Homebrew dependency
- added Windows 11 PowerShell-aware process handling, binary defaults, and config discovery
- added an explicit `cpanfile` so the dependency gate remains present even with core-only Perl dependencies

## Why it changed

Some OpenVPN tunnels require a password plus a six-digit 2FA suffix during reconnect. Without a skill-owned reconnect path, those tunnels break unattended recovery and force manual intervention after each disconnect.

## Proof

- Docker tests and 100% coverage passed
- installed runtime proof passed through the real `dashboard openvpn.*` command path with a fake `openvpn` executable

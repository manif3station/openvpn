# 2026-05-05 Windows Go Connect Fix

## Summary

Fixed the standalone Go mirror Windows connect path so it behaves more realistically when OpenVPN delays pid-file creation, and made failures point at the OpenVPN log.

## What Changed

- kept the raw `MFA` secret or static suffix in `~/.openvpn.env`
- clarified that `auth.txt` is a runtime helper that stores the generated current code for the active connect attempt only
- added `log_file` to the Go mirror JSON payload
- added log-path context to Windows launcher failure messages
- allowed the Go launcher to accept a live spawned OpenVPN pid when the Windows pid file is delayed

## Verification

- Docker Go tests passed
- Docker Go coverage passed at `100.0%` statements for `github.mf/manif3station/openvpn/go-version/mirror`
- Docker Windows-style build proof passed for `setup.exe`, `connect.exe`, `disconnect.exe`, and `noreconnect.exe`

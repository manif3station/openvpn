# openvpn Go Mirror

This folder contains a standalone Go mirror of the `openvpn` skill commands so the workflow can run without the Developer Dashboard runtime.

## Commands

- `setup`
- `connect`
- `disconnect`
- `noreconnect`

## Source-Only Policy

This folder keeps source code and build scripts only.

Generated binaries such as `cli/setup.exe` or `cli/connect.exe` are build outputs and are intentionally ignored by Git.

## Environment Contract

The Go mirror reads:

```text
~/.openvpn.env
```

Supported keys:

- `USERNAME`
- `PASSWORD`
- `MFA`
- `CONFIG`
- `OPENVPN_BIN`

Legacy compatibility keys are still accepted on read:

- `OPENVPN_USERNAME`
- `OPENVPN_PASSWORD`
- `OPENVPN_2FA`
- `OPENVPN_CONFIG`

## Build The Windows Mirror

From macOS or Linux:

```bash
cd ~/projects/skills/skills/openvpn/go-version
./build.sh
```

From Windows PowerShell:

```powershell
Set-Location ~/projects/skills/skills/openvpn/go-version
./build.ps1
```

Both scripts generate:

- `cli/setup.exe`
- `cli/connect.exe`
- `cli/disconnect.exe`
- `cli/noreconnect.exe`

## Run Without Developer Dashboard

Direct Go subcommand form:

```bash
cd ~/projects/skills/skills/openvpn/go-version
go run . setup
go run . connect --auto
go run . disconnect
go run . noreconnect
```

Built Windows mirror form:

```powershell
./cli/setup.exe -u alice -p 'secret-password' -2fa JBSWY3DPEHPK3PXP -c '~/openvpn/config/work.ovpn'
./cli/connect.exe --auto
./cli/noreconnect.exe
./cli/disconnect.exe
```

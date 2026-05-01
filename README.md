# openvpn

## Description

`openvpn` is a Developer Dashboard skill that keeps a username/password OpenVPN session usable when the VPN gateway expects a 2FA six-digit suffix on the password during reconnect.

## Value

It saves the user from manually re-entering the username, password, and current 2FA suffix every time the OpenVPN tunnel drops and needs to come back.

## Problem It Solves

Some OpenVPN deployments accept a normal username but require the password field to end with a changing six-digit 2FA value. When the tunnel drops, a plain OpenVPN reconnect path cannot complete by itself because it needs that extra suffix again. That breaks unattended reconnect and forces the user back into manual recovery.

## What It Does To Solve It

This skill:

- stores the OpenVPN username and password in `~/.openvpn.env`
- optionally stores a 2FA value in the same file
- treats a six-digit `OPENVPN_2FA` value as a static suffix
- treats any other `OPENVPN_2FA` value as a TOTP secret or `otpauth://` URI and generates the current six-digit code for each connection attempt
- writes an `auth-user-pass` file on demand for OpenVPN
- monitors the tunnel through a DD collector
- attempts reconnect automatically after disconnect when auto reconnect is enabled
- disables auto reconnect after five failed retry attempts
- lets the user disable reconnect manually with `dashboard openvpn.noreconnect`
- lets the user force a one-off reconnect with `dashboard openvpn.connect`
- lets the user force and re-enable managed reconnect with `dashboard openvpn.connect --auto`
- stops the tunnel and disables reconnect with `dashboard openvpn.disconnect`

## Developer Dashboard Feature Added

This skill adds:

- `dashboard openvpn.setup`
- `dashboard openvpn.connect`
- `dashboard openvpn.disconnect`
- `dashboard openvpn.noreconnect`
- a collector declared in `config/config.json`
- an indicator template that renders as `OVPN?`, `OVPN+`, `OVPN!`, `OVPN-`, or `OVPNx`

## Installation

Install from the skill repository:

```bash
dashboard skills install git@github.mf:manif3station/openvpn.git
```

For local development in this workspace:

```bash
dashboard skills install ~/projects/skills/skills/openvpn
```

## Runtime Dependencies

This skill depends on the `openvpn` executable:

- Debian or Ubuntu: `aptfile`
- macOS: `brewfile`

## How To Use It

Interactive setup:

```bash
dashboard openvpn.setup
```

Non-interactive setup:

```bash
dashboard openvpn.setup -u alice -p 'secret-password' -2fa JBSWY3DPEHPK3PXP
```

One-off connect without turning automatic reconnect back on:

```bash
dashboard openvpn.connect
```

Connect and enable automatic reconnect:

```bash
dashboard openvpn.connect --auto
```

Disable automatic reconnect without disconnecting:

```bash
dashboard openvpn.noreconnect
```

Disconnect and disable automatic reconnect:

```bash
dashboard openvpn.disconnect
```

Collector mode used by DD:

```bash
dashboard openvpn.connect --collector
```

## Setup File

The skill stores user-managed values in:

```text
~/.openvpn.env
```

Supported variables:

- `OPENVPN_USERNAME`
- `OPENVPN_PASSWORD`
- `OPENVPN_2FA`
- `OPENVPN_CONFIG`
- `OPENVPN_BIN`

`OPENVPN_CONFIG` is optional. If it is not present, the skill looks for one `.ovpn` file in these places:

- `~/.openvpn/config.ovpn`
- `~/.openvpn/client.ovpn`
- `~/.config/openvpn/client.ovpn`
- `~/.config/openvpn/config.ovpn`
- the first `*.ovpn` file under `~/.openvpn/`
- the first `*.ovpn` file under `~/.config/openvpn/`

## Indicator And Collector Behavior

The shipped collector config is in `config/config.json` and runs:

```text
dashboard openvpn.connect --collector
```

Before setup is complete, the collector returns a nonzero result with status icon `?`, so the DD indicator becomes red and renders `OVPN?`.

After setup:

- `OVPN+` means the tunnel is connected
- `OVPN!` means reconnect is being attempted or the latest attempt failed
- `OVPN-` means reconnect has been disabled
- `OVPNx` is used by the explicit disconnect command result

If the user wants a different interval or icon, they can override the collector in `~/.developer-dashboard/config/config.json`.

## Normal Cases

```text
Run `dashboard openvpn.setup` once, then let the collector maintain the tunnel in the background.
```

```text
Use `dashboard openvpn.connect --auto` after investigation if reconnect had been disabled after five failed retry attempts.
```

```text
Use `dashboard openvpn.connect` for a one-off connection attempt when you do not want the collector to keep retrying afterward.
```

## Edge Cases

```text
If `~/.openvpn.env` is incomplete or missing, the collector returns `OVPN?` and exits nonzero so DD shows a red indicator instead of pretending the tunnel is healthy.
```

```text
If no OpenVPN config file can be found automatically, set `OPENVPN_CONFIG=~/path/to/profile.ovpn` in `~/.openvpn.env`.
```

```text
If `OPENVPN_2FA` is exactly six digits, the skill uses it as a literal suffix.
```

```text
If `OPENVPN_2FA` is not six digits, the skill treats it as a TOTP secret and generates a fresh six-digit code for each connect attempt.
```

```text
If `OPENVPN_2FA` is an `otpauth://` URI, the skill extracts the `secret=` value and generates the current six-digit code from that secret.
```

```text
If reconnect fails five times in a row, the collector disables reconnect and leaves the tunnel for manual investigation until the user runs `dashboard openvpn.connect --auto`.
```

## Documentation

- `docs/overview.md`
- `docs/usage.md`
- `docs/changes/2026-05-01-initial-release.md`

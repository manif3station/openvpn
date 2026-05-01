# Usage

## Install

```bash
dashboard skills install git@github.mf:manif3station/openvpn.git
```

Local workspace install:

```bash
dashboard skills install ~/projects/skills/skills/openvpn
```

Windows 11 PowerShell install:

```powershell
dashboard skills install git@github.mf:manif3station/openvpn.git
```

Perl dependency manifest:

```text
cpanfile
```

## Setup

Interactive:

```bash
dashboard openvpn.setup
```

Promptless setup:

```bash
dashboard openvpn.setup -u alice -p 'secret-password' -2fa JBSWY3DPEHPK3PXP
```

The skill writes:

```text
~/.openvpn.env
```

If `openvpn` is not on `PATH`, add this too:

```text
OPENVPN_BIN=~/path/to/openvpn
```

On Windows 11 PowerShell, `OPENVPN_BIN` will usually point at an `openvpn.exe` path.

## Connection Commands

One-off connection:

```bash
dashboard openvpn.connect
```

One-off connection with reconnect enabled:

```bash
dashboard openvpn.connect --auto
```

Collector path:

```bash
dashboard openvpn.connect --collector
```

Disable reconnect:

```bash
dashboard openvpn.noreconnect
```

Disconnect and disable reconnect:

```bash
dashboard openvpn.disconnect
```

## Collector Behavior

The shipped collector config runs every 10 seconds and uses:

```json
{
  "collectors": [
    {
      "name": "connector",
      "command": "dashboard openvpn.connect --collector",
      "cwd": "home",
      "interval": 10,
      "indicator": {
        "icon": "OVPN[% status_icon %]"
      }
    }
  ]
}
```

If you want a different interval or icon, override that collector in:

```text
~/.developer-dashboard/config/config.json
```

## Config Discovery

If `OPENVPN_CONFIG` is not present in `~/.openvpn.env`, the skill tries these paths:

- `~/.openvpn/config.ovpn`
- `~/.openvpn/client.ovpn`
- `~/.config/openvpn/client.ovpn`
- `~/.config/openvpn/config.ovpn`
- the first `*.ovpn` file under `~/.openvpn/`
- the first `*.ovpn` file under `~/.config/openvpn/`

## Practical Notes

- use a six-digit `OPENVPN_2FA` only if your VPN actually expects a fixed suffix
- use a non-six-digit `OPENVPN_2FA` value for a TOTP secret so the skill can generate the current six-digit code
- use an `otpauth://` URI in `OPENVPN_2FA` if that is how your VPN team shares the TOTP secret
- the skill does not install `openvpn` for you; keep your existing host install or set `OPENVPN_BIN`
- on Windows 11 PowerShell, hidden password prompts fall back to visible prompts, so use non-interactive setup if you want to avoid typing secrets on screen
- use `dashboard openvpn.connect --auto` after a five-failure lockout to re-enable reconnect attempts
- use `dashboard openvpn.connect` if you want one connection attempt without turning reconnect back on

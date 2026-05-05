# 2026-05-05 Windows Auto-Login Layout

- added the short `USERNAME`, `PASSWORD`, `MFA`, and `CONFIG` env contract to `~/.openvpn.env`
- kept read compatibility with the older `OPENVPN_USERNAME`, `OPENVPN_PASSWORD`, `OPENVPN_2FA`, and `OPENVPN_CONFIG` keys
- added `-c` and `--config` to `dashboard openvpn.setup`
- made `~/openvpn/config/` a first-class config discovery location
- moved the default Windows runtime helper directory to `~/openvpn/config/dd-runtime`
- added `--auth-retry nointeract` to the managed OpenVPN launcher path so the CLI flow does not reopen an auth popup

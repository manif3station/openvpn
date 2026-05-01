# Testing

## Policy

- tests run only inside Docker
- the shared test container definition lives at the workspace root
- this skill keeps its test files in `t/`

## Commands

```bash
docker compose -f ~/projects/skills/docker-compose.testing.yml run --rm perl-test bash -lc 'cd /workspace/skills/openvpn && prove -lr t'
docker compose -f ~/projects/skills/docker-compose.testing.yml run --rm perl-test bash -lc 'cd /workspace/skills/openvpn && cover -delete && HARNESS_PERL_SWITCHES=-MDevel::Cover prove -lr t && cover -report text -select_re "^lib/" -coverage statement -coverage subroutine && rm -rf /workspace/skills/openvpn/cover_db'
```

## Latest Result

- Docker functional tests passed: `Files=7, Tests=98`
- Docker functional tests passed: `Files=8, Tests=129`
- Docker coverage passed:
  - `lib/OpenVPN/Manager.pm` `100.0%` statement and `100.0%` subroutine
  - `lib/OpenVPN/Launcher.pm` `100.0%` statement and `100.0%` subroutine
  - `lib/OpenVPN/TOTP.pm` `100.0%` statement and `100.0%` subroutine
- Installed runtime proof passed through the real `dashboard openvpn.*` entrypoints with a fake `openvpn` executable
- Simulated Windows 11 PowerShell paths passed in Docker through launcher tests for:
  - `openvpn.exe` default binary selection
  - Windows pid lookup and taskkill-style stop handling
  - Windows config discovery under `ProgramFiles` and user-profile config paths
  - visible-prompt fallback for hidden secret entry
- Proven runtime states:
  - `dashboard openvpn.connect --collector` before setup returned `status=not_setup`, `status_icon=?`, and a nonzero exit code
  - `dashboard openvpn.setup -u ... -p ... -2fa 'otpauth://...'` wrote `~/.openvpn.env`
  - `dashboard openvpn.connect --auto` returned `status=connected`
  - `dashboard openvpn.noreconnect` returned `status=reconnect_disabled`
  - `dashboard openvpn.disconnect` returned `status=disconnected`
  - `dashboard openvpn.connect --collector` after disconnect with reconnect disabled returned `status=reconnect_disabled` and a nonzero exit code
- `cover_db` was removed after verification

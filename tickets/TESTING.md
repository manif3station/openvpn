# Testing

## Policy

- tests run only inside Docker
- the shared test container definition lives at the workspace root
- this skill keeps its test files in `t/`

## Commands

```bash
docker compose -f ~/projects/skills/docker-compose.testing.yml run --rm perl-test bash -lc 'cd /workspace/skills/openvpn && prove -lvr t'
docker compose -f ~/projects/skills/docker-compose.testing.yml run --rm perl-test bash -lc 'cd /workspace/skills/openvpn && rm -rf cover_db /workspace/cover_db/* && HARNESS_PERL_SWITCHES=-MDevel::Cover prove -lvr t && cover -report text'
docker run --rm -v ~/projects/skills/skills/openvpn/go-version:/work -w /work golang:1.22-bookworm bash -lc '/usr/local/go/bin/go test ./mirror -coverprofile=coverage.out && /usr/local/go/bin/go tool cover -func=coverage.out'
```

## Latest Result

- Docker functional tests passed: `Files=8, Tests=144`
- Docker coverage passed:
  - `lib/OpenVPN/Manager.pm` `100.0%` statement and `100.0%` subroutine
  - `lib/OpenVPN/Launcher.pm` `100.0%` statement and `100.0%` subroutine
  - `lib/OpenVPN/TOTP.pm` `100.0%` statement and `100.0%` subroutine
- `cpanfile` is present and explicitly records the core Perl modules used by the skill, with skill-local modules noted separately
- Simulated Windows 11 PowerShell paths passed in Docker through launcher tests for:
  - `openvpn.exe` default binary selection
  - `~/openvpn/config/dd-runtime` runtime helper placement
  - `~/openvpn/config/*.ovpn` discovery before older fallback paths
  - Windows pid lookup and taskkill-style stop handling
  - Windows config discovery under `ProgramFiles` and user-profile config paths
  - visible-prompt fallback for hidden secret entry
  - `--auth-retry nointeract` on the managed launcher command
- PowerShell runtime proof passed in Docker through the real skill CLI wrappers:
  - `perl cli/connect --collector` returned `status=not_setup` with a nonzero exit code
  - `perl cli/setup -u ... -p ... -2fa 'otpauth://...' -c '~/openvpn/config/work.ovpn'` returned `two_factor=totp`
  - `perl cli/connect --auto` returned `status=connected`
  - `perl cli/noreconnect` returned `status=reconnect_disabled`
  - `perl cli/disconnect` returned `status=disconnected`
  - `perl cli/connect --collector` after disconnect with reconnect disabled returned `status=reconnect_disabled` with a nonzero exit code
- Proven runtime states:
  - `dashboard openvpn.connect --collector` before setup returned `status=not_setup`, `status_icon=?`, and a nonzero exit code
  - `dashboard openvpn.setup -u ... -p ... -2fa 'otpauth://...' -c '~/openvpn/config/work.ovpn'` wrote `~/.openvpn.env`
  - `~/.openvpn.env` now stores the canonical keys `USERNAME`, `PASSWORD`, `MFA`, and `CONFIG`
  - legacy `OPENVPN_USERNAME`, `OPENVPN_PASSWORD`, `OPENVPN_2FA`, and `OPENVPN_CONFIG` remain readable for backward compatibility
  - `dashboard openvpn.connect --auto` returned `status=connected`
  - `dashboard openvpn.noreconnect` returned `status=reconnect_disabled`
  - `dashboard openvpn.disconnect` returned `status=disconnected`
  - `dashboard openvpn.connect --collector` after disconnect with reconnect disabled returned `status=reconnect_disabled` and a nonzero exit code
- Host integration note:
  - `macdev` and `windev` were down for this ticket, so there is no live host integration proof for the new Windows-oriented layout in this release
  - Linux, macOS, and Windows code paths are covered by automated tests inside Docker
- `cover_db` was removed after verification
- Go mirror verification:
  - Docker Go tests passed: `ok github.mf/manif3station/openvpn/go-version/mirror`
  - Docker Go coverage passed: `100.0%` statements for `github.mf/manif3station/openvpn/go-version/mirror`
  - Docker Go Windows-style build proof passed:
    - `./build.sh` generated `connect.exe`, `disconnect.exe`, `noreconnect.exe`, and `setup.exe`
    - those generated files were deleted afterward so `go-version/` remained source-only
  - Go mirror Windows launcher fix proof passed in Docker:
    - delayed or missing pid-file paths were exercised
    - spawned-pid fallback on Windows was exercised
    - failure messages now include log-path context
    - result payloads now include `log_file`
  - generated `.exe` files were not kept in `go-version/`; the repo remains source-only for the standalone mirror
  - no live `macdev` or `windev` integration proof was required for this ticket

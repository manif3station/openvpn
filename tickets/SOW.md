# SOW

## SOW-001

Create a new `openvpn` skill that can capture OpenVPN credentials, optionally append a six-digit 2FA suffix, and keep the VPN tunnel connected through a DD collector-backed reconnect flow.

## Scope

- implement `dashboard openvpn.setup`
- implement `dashboard openvpn.connect`
- implement `dashboard openvpn.disconnect`
- implement `dashboard openvpn.noreconnect`
- store the user-managed setup file at `~/.openvpn.env`
- add a shipped collector config and indicator contract
- support Windows-friendly OpenVPN credential automation without repeated login prompts
- keep Linux and macOS behavior aligned with the same managed auth flow
- add a standalone Go mirror under `go-version/` so the workflow can run without the Developer Dashboard runtime
- verify the skill inside Docker
- complete documentation, commit, and push gates

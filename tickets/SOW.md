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
- verify the skill inside Docker
- complete documentation, commit, and push gates

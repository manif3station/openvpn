# Overview

`openvpn` is a DD operational skill for OpenVPN tunnels that need a password plus a six-digit 2FA suffix to reconnect.

The skill keeps the reconnect flow inside DD instead of pushing the user back into a manual recovery cycle every time the tunnel drops.

The main behavior is:

- `dashboard openvpn.setup` records the username, password, and optional 2FA secret in `~/.openvpn.env`
- `dashboard openvpn.connect` performs one connection attempt and leaves reconnect disabled afterward
- `dashboard openvpn.connect --auto` performs a connection attempt and enables managed reconnect
- `dashboard openvpn.connect --collector` is the collector path that reports indicator state and retries reconnect when allowed
- `dashboard openvpn.noreconnect` disables reconnect without tearing down the current process
- `dashboard openvpn.disconnect` disconnects and disables reconnect

The collector indicator starts as `OVPN?` before setup is complete. That produces a red DD indicator state on purpose so the user sees that the skill is not ready yet.

Once setup is complete, the collector watches the managed OpenVPN process state and reconnects after disconnect when auto reconnect is enabled. After five failed reconnect attempts, it disables reconnect and returns an alert state until the user investigates and runs `dashboard openvpn.connect --auto` again.

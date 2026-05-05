# Windows auth override order

## Problem

Some OpenVPN profiles already contain their own `auth-user-pass` directive. The standalone Windows mirror was generating a correct `auth.txt`, but the launch argument order still let the profile override the managed auth file. In that state OpenVPN started, then logged `Auth username is empty` and exited.

## Fix

The launcher contract now emits:

```text
--config <profile>
--auth-user-pass <generated-auth-file>
--auth-retry nointeract
```

That ordering is now aligned in both:

- the Perl `dashboard openvpn.*` launcher
- the standalone Go mirror under `go-version/`

## Proof

- Docker Perl functional tests passed
- Docker Perl coverage still passed at `100.0%` statement and `100.0%` subroutine for the production modules
- Docker Go tests passed
- Docker Go coverage stayed at `100.0%`
- launcher tests now assert that the managed auth override is positioned after the profile path

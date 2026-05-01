# Testing

## Policy

- tests run only inside Docker
- the shared test container definition lives at the workspace root
- this skill keeps its test files in `t/`

## Commands

```bash
docker compose -f ~/projects/skills/docker-compose.testing.yml run --rm perl-test bash -lc 'cd /workspace/skills/openvpn && prove -lr t'
docker compose -f ~/projects/skills/docker-compose.testing.yml run --rm perl-test bash -lc 'cd /workspace/skills/openvpn && cover -delete && HARNESS_PERL_SWITCHES=-MDevel::Cover prove -lr t && cover -report text -select_re "^lib/" -coverage statement -coverage subroutine'
```

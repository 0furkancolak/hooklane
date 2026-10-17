# Security policy

## Reporting a vulnerability

Please do not open a public issue. Use GitHub's private vulnerability
reporting for this repository (**Security → Report a vulnerability**) and
include:

- what an attacker can do, and what they need (an API key? network access?)
- steps or a proof of concept
- the version or commit you tested

You should get an acknowledgement within a few days. Fixes are released as
soon as they are ready, and the advisory credits you unless you prefer
otherwise.

## Supported versions

Until 1.0, only the latest release receives security fixes.

## Scope

Of particular interest:

- reaching internal addresses through webhook deliveries (SSRF), including
  DNS rebinding or redirects
- reading or changing another organization's data
- leaking API keys, endpoint secrets or payloads (in responses, logs or
  stored errors)
- forging or replaying webhook signatures

The security model, its assumptions and its known limits are described in
the README's [Security](README.md#security) section and in
[docs/deployment.md](docs/deployment.md#network-and-security-checklist).

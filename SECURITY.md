# Security Policy

The OpenFoundry maintainers take security seriously. This document explains how
to report a vulnerability, what we promise in return, and which versions of
OpenFoundry are eligible for security fixes.

> **Never disclose a suspected vulnerability through a public GitHub issue,
> pull request, discussion, or chat.**

## Reporting a vulnerability

Use **one** of the following private channels:

1. **Preferred — GitHub Security Advisories.**
   Open a private report at
   <https://github.com/open-foundry/open-foundry/security/advisories/new>.
   This keeps the report, our triage, and any fix coordinated in one place.

2. **Email.** Send an encrypted message to **`security@openfoundry.dev`**.
   Our PGP key fingerprint is published at
   <https://diocrafts.github.io/OpenFoundry/security/pgp.txt>.
   If you cannot use PGP, send an unencrypted message and we will reply with
   a secure channel.

Please **do not** contact individual maintainers privately about
vulnerabilities — it slows down triage and breaks the audit trail.

### What to include

A useful report contains, at minimum:

- The **affected component(s)** (service, library, proto, SDK, infra
  manifest) and version / commit SHA.
- A **clear description** of the issue and its impact.
- **Reproduction steps** or a minimal proof-of-concept.
- Any **logs, stack traces or screenshots** that help triage.
- Your **assessment of severity** (CVSS 3.1 vector if possible).
- Whether you are willing to be credited and how.

The more reproducible the report, the faster we can ship a fix.

## Our commitments

When you report in good faith through the channels above, we commit to:

| Stage | Target |
|-------|--------|
| Acknowledge receipt | within **3 business days** |
| Initial triage and severity assessment | within **7 business days** |
| Status update cadence | at least every **14 days** until resolution |
| Fix for **Critical** issues | within **30 days** of triage |
| Fix for **High** issues | within **60 days** of triage |
| Fix for **Medium / Low** issues | next scheduled minor release |
| Public disclosure | coordinated with the reporter, by default **90 days** after the initial report or 7 days after a fix is released, whichever is sooner |

We will keep you in the loop throughout, and we will credit you in the
advisory and in [`CHANGELOG.md`](CHANGELOG.md) unless you ask otherwise.

## Severity guidance

We use CVSS 3.1. As a rough internal guide:

- **Critical** — unauthenticated remote code execution; full tenant data
  exfiltration; authentication bypass affecting all users; secret material
  leak in logs / artefacts.
- **High** — authenticated privilege escalation across tenants; injection
  vulnerabilities in core data paths; significant denial of service against
  shared services.
- **Medium** — authenticated information disclosure limited to the same
  tenant; CSRF / SSRF requiring user interaction; logic flaws bypassing
  non-critical controls.
- **Low** — issues with very limited impact, hardening gaps, missing defence
  in depth.

## Scope

In scope:

- All Go services under [`services/`](services/) and shared Go packages under
  [`libs/`](libs/).
- Protobuf contracts under [`proto/`](proto/) and the generated SDKs in
  [`sdks/`](sdks/).
- The web frontend under [`apps/web/`](apps/web/).
- Default deployment manifests under [`infra/`](infra/) (Helm charts,
  Terraform modules, Docker Compose files).
- Official container images published from this repository.
- The official documentation site at
  <https://diocrafts.github.io/OpenFoundry/>.

Out of scope (please **do not** report these as security issues):

- Vulnerabilities in third-party dependencies that have not been exploited
  through OpenFoundry — open a regular issue and we will bump the dependency.
- Issues that require a malicious operator with full administrative access to
  the cluster running OpenFoundry.
- Self-XSS, missing security headers on the marketing site, clickjacking on
  pages with no sensitive actions.
- Volumetric DDoS or rate-limit bypass on public, unauthenticated endpoints
  designed for high traffic (e.g. `/health`).
- Findings produced solely by automated scanners with no demonstrated impact.

## Supported versions

OpenFoundry follows semantic versioning at the platform level. Security fixes
are backported only to supported versions:

| Version line | Status | Security fixes |
|--------------|--------|----------------|
| `main` | active development | yes |
| Latest minor (`x.Y.z`) | supported | yes |
| Previous minor (`x.Y-1.z`) | maintenance | yes, until next minor + 30 days |
| Older versions | end of life | no |

Until the project reaches a `1.0.0` release, only `main` and the latest
tagged release line are supported.

## Safe-harbour

We will not pursue legal action against researchers who:

- Make a good-faith effort to comply with this policy.
- Avoid privacy violations, data destruction, and service disruption.
- Do not exploit a vulnerability beyond what is necessary to confirm it.
- Give us a reasonable amount of time to fix the issue before any public
  disclosure.

If in doubt, contact us first — we would rather hear from you than not.

## Hardening and supply-chain practices

We try to make secure contributions easy:

- Go dependency advisories are checked via
  [`govulncheck`](.github/workflows/security-audit.yml).
- Container images are built from pinned bases and scanned in CI.
- Protobuf, OpenAPI, and generated SDK changes go through drift checks in CI
  to prevent silent contract breaks.
- Production manifests under [`infra/`](infra/) default to least privilege,
  network policies, and read-only root filesystems.

If you spot a gap in any of the above, please open a regular issue.

---

Thank you for helping keep OpenFoundry and its users safe.

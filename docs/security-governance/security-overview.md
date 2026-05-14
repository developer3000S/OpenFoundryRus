# OpenFoundry security overview

> Landing page for OpenFoundry's security and governance surfaces. The
> goal is to give administrators, operators, developers, and reviewers a
> single starting point for "how is this platform protected?" before
> they descend into a specific subsystem.
>
> **Scope boundary.** Everything below is OpenFoundry-native. Where
> behavior is intentionally modeled on a Foundry concept, the section
> cites the public Palantir page that justifies the parity feature as
> required by the
> [Foundry public-docs parity policy](../reference/foundry-public-docs-parity-policy.md).
> No Palantir branding, screenshots, or private source is reused.

## How security is layered

OpenFoundry composes seven independent control layers. None of them is
sufficient on its own — every read, write, action, or export must pass
through the **conjunction** of the layers that apply.

| # | Layer | Question it answers | Primary owner |
|---|---|---|---|
| 1 | **Identity** | Who is the caller? | [`services/identity-federation-service`](../../services/identity-federation-service/) |
| 2 | **Organization / space membership** | Is the caller inside the tenancy boundary that owns this resource? | [`services/tenancy-organizations-service`](../../services/tenancy-organizations-service/) |
| 3 | **Discretionary roles** | Does the caller hold a role on this resource (Owner / Editor / Viewer / Discoverer / custom)? | [`services/authorization-policy-service`](../../services/authorization-policy-service/) + [`libs/auth-middleware`](../../libs/auth-middleware/) |
| 4 | **Mandatory markings** | Does the caller satisfy **all** markings that apply to this resource (directly, by inheritance, or through lineage)? | `authorization-policy-service` (planned `marking-service` carve-out — see [checklist `SG.11`–`SG.15`](../migration/foundry-security-governance-1to1-checklist.md)) |
| 5 | **Restricted views and row-level policy** | Which rows / columns / objects is the caller allowed to see inside the resource? | `authorization-policy-service` (`internal/handlers/restricted_views.go`) |
| 6 | **Scoped session** | Of the markings the caller normally satisfies, which subset is active right now? | Planned scoped-session preset on `tenancy-organizations-service` (see [checklist `SG.24`–`SG.25`](../migration/foundry-security-governance-1to1-checklist.md)) |
| 7 | **Network egress and export controls** | Can data leave the platform via this destination / channel? | Planned `egress-governance-service` (see [checklist `SG.34`–`SG.35`](../migration/foundry-security-governance-1to1-checklist.md)); audit emission in [`libs/audit-trail`](../../libs/audit-trail/) |

Every decision is recorded by the audit subsystem
([`services/audit-compliance-service`](../../services/audit-compliance-service/) +
[`services/audit-sink`](../../services/audit-sink/)) so that authorization,
markings, retention, and egress can be reviewed after the fact.

> Parity reference:
> [Security overview](https://www.palantir.com/docs/foundry/security/overview/) ·
> [Checking permissions](https://www.palantir.com/docs/foundry/security/checking-permissions/).

## The seven control layers in detail

### 1. Identity

OpenFoundry treats every caller as either a **user** (human, federated
through an external identity provider) or a **service user** (long-lived
machine identity tied to an OAuth client). Both kinds carry an immutable
ID, a primary organization, a realm, and a claim bundle.

- Login, JWT issuance, JWKS rotation, MFA, WebAuthn, OIDC, SAML, and
  SCIM are all handled by
  [`services/identity-federation-service`](../../services/identity-federation-service/).
- Claims reach handlers through [`libs/auth-middleware`](../../libs/auth-middleware/);
  handlers must never parse JWTs themselves (see [CLAUDE.md](../../CLAUDE.md) §"Conventions").
- Service users for confidential OAuth clients are stored alongside
  human users and have independent role grants (planned in
  [checklist `SG.33`](../migration/foundry-security-governance-1to1-checklist.md)).

Detailed surface: [Identity and access](./identity-and-access.md).

> Parity reference:
> [Authentication overview](https://www.palantir.com/docs/foundry/authentication/overview/) ·
> [Protecting identity](https://www.palantir.com/docs/foundry/security/single-sign-on-security/) ·
> [Users and groups](https://www.palantir.com/docs/foundry/security/users-and-groups/).

### 2. Organizations and spaces

An **organization** is a strict membership boundary. Users discover
resources only inside organizations they belong to. A **space** is a
store/administration boundary inside an organization, used for projects,
retention scope, and quotas.

- Enrollments, organizations, spaces, and guest memberships live in
  [`services/tenancy-organizations-service`](../../services/tenancy-organizations-service/).
  As of SG.2 (2026-05-14) the service persists organization metadata,
  settings and quotas, plus four supporting tables: `tenancy_enrollments`
  (user→org), `tenancy_organization_admins`, `tenancy_organization_guests`,
  and `tenancy_spaces` (Foundry-style spaces — distinct from the federation
  `nexus_spaces`).
- The Control Panel UI in [`apps/web/src/routes/control-panel/`](../../apps/web/src/routes/control-panel/)
  exposes organization-level settings, with a dedicated
  [`/control-panel/tenancy`](../../apps/web/src/routes/control-panel/TenancyPage.tsx)
  surface for administrators, guests, spaces and the membership probe.
- Resource discovery enforcement is layered:
  1. JWT claims carry the caller's organization (`Claims.OrgID`) and
     [`Claims.AllowsOrgID`](../../libs/auth-middleware/claims.go) is the
     in-process check used by every protected handler.
  2. For persistent admin/guest grants beyond what's in the token, the
     [`GET /api/v1/organizations/{id}/membership`](../../services/tenancy-organizations-service/internal/handlers/organization_governance.go)
     endpoint consults the four tables above and reports whether the
     caller is a primary member, an admin, or an active guest.
- Cross-organization collaboration (B2B / B2C / consumer mode) is
  governed by guest membership rules — additional surface tracked under
  [checklist `SG.41`–`SG.42`](../migration/foundry-security-governance-1to1-checklist.md).

> Parity reference:
> [Organizations and spaces](https://www.palantir.com/docs/foundry/security/orgs-and-spaces/) ·
> [Enrollments and organizations permissions](https://www.palantir.com/docs/foundry/administration/enrollments-and-organizations-permissions/).

### 3. Discretionary roles

Roles are **bundles of operations** granted on a project, folder,
resource, or platform context. Default roles follow the public Foundry
shapes — Owner, Editor, Viewer, Discoverer — and OpenFoundry supports
context-specific role sets on top.

- The RBAC primitives live in
  [`services/identity-federation-service/internal/domain/rbac`](../../services/identity-federation-service/internal/domain/rbac/).
- Role / operation evaluation runs in
  [`services/authorization-policy-service`](../../services/authorization-policy-service/)
  and is distributed to each service as an in-process bundle (see
  [Policy bundles in-process](./policy-bundles.md)).
- Grantors **cannot delegate above their own role level**; this rule
  is checked at write time, not only at read time
  (see [checklist `SG.7`](../migration/foundry-security-governance-1to1-checklist.md)).

Detailed surface: [Policies and authorization](./policies-and-authorization.md).

> Parity reference:
> [Projects and roles](https://www.palantir.com/docs/foundry/security/projects-and-roles) ·
> [Manage roles](https://www.palantir.com/docs/foundry/platform-security-management/manage-roles/).

### 4. Mandatory markings

Markings are mandatory access controls (MAC). Unlike roles, **applying
a marking does not grant access**: it only declares the requirement.
A user can read a marked resource only if they are a **member** of
every marking that applies — directly, through project/folder
inheritance, or through dataset lineage propagation.

- Marking categories, marking metadata, and member grants are owned by
  `authorization-policy-service` today; a dedicated `marking-service`
  carve-out is planned (see
  [checklist `SG.11`–`SG.15`](../migration/foundry-security-governance-1to1-checklist.md)).
- `apply`, `remove`, `manage`, and `member` are **four distinct
  permissions**. `apply` and `manage` deliberately do not imply access.
- Removing a marking from a derived resource requires both
  `remove marking` and a permission to expand access — this prevents
  silent declassification.

> Parity reference:
> [Markings](https://www.palantir.com/docs/foundry/security/markings/) ·
> [Manage markings](https://www.palantir.com/docs/foundry/platform-security-management/manage-markings/).

### 5. Restricted views and row-level policy

Restricted views are dataset-backed resources that expose a filtered
projection of an input dataset. A view's **granular policy** combines
user attributes, group membership, organization IDs, column values,
and constants into a row-level predicate evaluated at query time.

- View definitions live in
  [`services/authorization-policy-service/internal/handlers/restricted_views.go`](../../services/authorization-policy-service/internal/handlers/restricted_views.go).
- Query rewrite is performed by the data-plane services that own the
  storage layer (Iceberg catalog, object database, SQL/BI gateway),
  consuming the same policy bundle that the PDP would have used.
- Marking-backed restricted views require the caller to satisfy
  `string[]` marking IDs in **each row** in addition to all
  organization, role, and scoped-session checks
  (see [checklist `SG.22`](../migration/foundry-security-governance-1to1-checklist.md)).

Detailed surface: [Restricted views and data controls](./restricted-views-and-data-controls.md).

> Parity reference:
> [Restricted views](https://www.palantir.com/docs/foundry/security/restricted-views) ·
> [Manage restricted views](https://www.palantir.com/docs/foundry/platform-security-management/manage-restricted-views/).

### 6. Audit and traceability

Every security-relevant decision — login, role grant, marking apply,
restricted-view query, action submission, egress import, retention
deletion — emits a normalized audit event.

- Producers use [`libs/audit-trail`](../../libs/audit-trail/) so the
  event shape is uniform across services.
- [`services/audit-compliance-service`](../../services/audit-compliance-service/)
  owns the platform audit ledger, the retention policy registry, and
  the lineage deletion subsystem.
- [`services/audit-sink`](../../services/audit-sink/) is the
  Kafka → Iceberg consumer for the `audit.events.v1` stream and is the
  long-term archive used by SIEM exports.
- Audit data is itself sensitive: access to audit datasets is gated by
  separate permissions, not by ordinary project roles.

Detailed surface: [Audit and traceability](./audit-and-traceability.md) ·
[Audit model](./audit-model/).

> Parity reference:
> [Audit logs overview](https://www.palantir.com/docs/foundry/security/audit-logs-overview) ·
> [Monitor audit logs](https://www.palantir.com/docs/foundry/security/monitor-audit-logs) ·
> [Audit log categories](https://www.palantir.com/docs/foundry/security/audit-log-categories).

### 7. Retention, egress, and notification controls

Three additional surfaces protect data after it has been authorized:

- **Retention policies** schedule transaction-level deletion of
  datasets. Recommended, custom, and legacy policy types are scoped to
  a space and can mark current-view transactions for deletion. The
  retention engine in
  [`services/audit-compliance-service`](../../services/audit-compliance-service/)
  records irreversible deletion warnings and execution history
  (see [checklist `SG.36`–`SG.37`](../migration/foundry-security-governance-1to1-checklist.md)).
- **Network egress policies** govern how data leaves the platform.
  Direct, agent-proxy, and same-region bucket egress all require an
  explicit import grant on the caller's workload, an `active` policy
  state, and an audit trail for every attach / use / revoke / pause
  event (planned `egress-governance-service` — see
  [checklist `SG.34`–`SG.35`](../migration/foundry-security-governance-1to1-checklist.md)).
- **Email content redaction** redacts notification payloads by default
  and only sends unredacted content under an explicit
  administrator-controlled allowlist
  (see [checklist `SG.38`](../migration/foundry-security-governance-1to1-checklist.md)).

> Parity reference:
> [Retention overview](https://www.palantir.com/docs/foundry/retention/overview/) ·
> [Configure network egress](https://www.palantir.com/docs/foundry/administration/configure-egress/) ·
> [Email content redaction](https://www.palantir.com/docs/foundry/email/email-content-redaction/).

## Resource model

Persisted state uses OpenFoundry-canonical IDs (no Palantir RID formats
required). Compatibility aliases may be accepted at API boundaries and
are decoded before storage. The full mapping from public Foundry
concepts to OpenFoundry resources is the
[Target OpenFoundry resource model table](../migration/foundry-security-governance-1to1-checklist.md#target-openfoundry-resource-model)
in the security and governance parity checklist.

## Where to go next

| If you want to… | Start here |
|---|---|
| Configure SSO, MFA, users, groups, sessions, or tokens | [Identity and access](./identity-and-access.md) |
| Understand the policy decision model (RBAC + ABAC + restricted views) | [Policies and authorization](./policies-and-authorization.md) |
| Understand how policies reach each data-plane service | [Policy bundles in-process](./policy-bundles.md) |
| Configure row-level / column-level access on a dataset | [Restricted views and data controls](./restricted-views-and-data-controls.md) |
| See the audit event shape and downstream consumers | [Audit and traceability](./audit-and-traceability.md) · [Audit model](./audit-model/) |
| See how user attributes feed authorization | [ABAC and CBAC model](./abac-and-cbac-model/) |
| Walk through a permission evaluation | [Policy evaluation flows](./policy-evaluation-flows/) |
| Understand who owns what | [Shared responsibility model](./shared-responsibility-model.md) |
| See the full parity backlog | [Security & governance parity checklist](../migration/foundry-security-governance-1to1-checklist.md) |

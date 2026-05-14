# OpenFoundry shared security responsibility model

> Whose job is which control? This page draws the line between
> **platform operators** (the team that runs an OpenFoundry deployment)
> and **tenants** (organization administrators, project owners,
> security officers, and end users inside an enrollment).
>
> The model is layered: lower layers are operator-owned; higher layers
> are progressively tenant-owned. **Every layer remains the operator's
> responsibility for *availability* and *correct enforcement of the
> tenant's configuration*; the tenant is responsible for *choosing the
> right configuration* and for the data and identities they bring.**
>
> Parity reference:
> [Shared security responsibility model](https://www.palantir.com/docs/foundry/security/shared-security-responsibility-model) ·
> [Protecting your self-hosted Foundry installation](https://www.palantir.com/docs/foundry/security/protect-foundry-installation).

## Roles in this document

| Role | Who | What they do |
|---|---|---|
| **Platform operator** | The team running an OpenFoundry deployment (cluster, services, datastores). For SaaS this is a single ops team; for self-hosted this is the deploying organization's infra/security team. | Provision and patch the cluster, run the OpenFoundry services, manage secrets/certificates, deliver audit data, keep upgrades green. |
| **Enrollment administrator** | The principal with the platform-level "enrollment admin" operation set. | Configures enrollments, top-level identity providers, retention defaults, egress allow-list, audit delivery, third-party app review policy. |
| **Organization administrator** | An admin inside one organization. | Manages org-level users/groups, organization-scoped IdP mappings, app access, scoped sessions, marking categories owned by the org, member discovery. |
| **Project owner** | A user with `Owner` on a project. | Configures default project role, role grants, project markings, project references, project-level audit visibility. |
| **Marking administrator** | A user granted `manage` on a marking or category. | Creates / edits the marking, grants `member` / `apply` / `remove`. Does **not** automatically have `member` (so does not necessarily have access to the data). |
| **Security officer / reviewer** | A user holding audit and security-monitoring operations. | Reviews audit logs, security findings, access reviews, egress approvals, retention approvals. |
| **End user / developer** | Any authenticated user. | Uses the platform under the constraints above; requests access via the access-request workflow; manages their own API tokens within policy. |

## Responsibility matrix

`O` = platform operator, `T` = tenant administrator (one of the
tenant-side roles above), `S` = shared. "Shared" means the operator
provides the enforcement and the tenant chooses the configuration.

| Control area | Provision | Configure | Operate | Audit & review |
|---|:---:|:---:|:---:|:---:|
| Cluster, OS, datastores, network plane | O | O | O | O |
| TLS certificates, JWKS signing keys, secrets in vault | O | O | O | O |
| Service deployment, upgrades, backups, DR | O | O | O | O |
| Identity provider integration (SAML / OIDC) | O | T | O | S |
| User lifecycle (preregister, activate, disable) | O | T | T | T |
| Group lifecycle (internal / external / rule-based) | O | T | T | T |
| Organization & space topology | O | T | T | T |
| Roles, role sets, custom operations | O | T | T | T |
| Marking categories & markings | O | T | T | T |
| Marking apply / remove on resources | O | T | T | T |
| Restricted views, granular policies | O | T | T | T |
| Scoped session presets | O | T | T | T |
| Project templates, file access presets | O | T | T | T |
| Application access policies | O | T | T | T |
| User & group discovery (privacy) | O | T | T | T |
| OAuth third-party app review & enablement | O | T | T | T |
| API tokens (per user) | O | — | T | T |
| Service users & client credentials | O | T | T | T |
| Network egress policies | O | T | T | T |
| Retention policies (recommended / custom / legacy) | O | T | T | T |
| Email content redaction settings | O | T | T | T |
| Audit log ingestion & retention | O | T | O | T |
| Audit delivery (SIEM API, datasets) | O | T | O | T |
| Security monitoring queries & dashboards | O | T | T | T |
| Access reviews & recertification | O | T | T | T |
| Data classification (which datasets, which markings) | — | T | T | T |
| User attribute correctness inside the IdP | — | T | T | T |
| Application content & data published on the platform | — | T | T | T |

## What the platform operator owns

These are the controls a tenant cannot override and must trust the
operator to provide:

- The cluster, the network plane, the storage backends (Postgres,
  Cassandra, Kafka, Iceberg-backed object store), and the secret /
  certificate vault.
- The service binaries and the policy engine. The published
  authorization decision must reflect the tenant's configuration —
  changing a service binary to ignore a marking is an operator
  responsibility, not a tenant one.
- Patching, upgrades, vulnerability response, host hardening (see
  [checklist `SG.54`](../migration/foundry-security-governance-1to1-checklist.md)).
- Continuous availability of the audit pipeline:
  [`libs/audit-trail`](../../libs/audit-trail/) →
  [`services/audit-compliance-service`](../../services/audit-compliance-service/) →
  [`services/audit-sink`](../../services/audit-sink/). Tenants choose
  *what* to capture and *where* to deliver it; operators guarantee that
  the pipeline runs and is tamper-evident.
- Secret management for OAuth client credentials, signing keys, IdP
  certificates. Tenants supply public material (metadata, JWKS URLs);
  operators keep the private material safe and rotate it.
- Multi-tenant isolation: an organization's resources, audit events,
  tokens, and policies must not leak to another organization through a
  service bug, cache, or shared queue.

## What the tenant owns

These responsibilities cannot be assumed by the operator:

- **The identity source of truth.** The accuracy of user attributes,
  group memberships, employment status, clearance levels, and IdP
  attribute mappings is the tenant's responsibility. A stale or
  over-broad group is a tenant problem, not a service problem.
- **The data brought onto the platform.** Datasets, media, objects,
  and AI prompts uploaded to OpenFoundry remain the tenant's data.
  Choosing what is sensitive — and assigning the right markings,
  restricted views, retention policies, and egress restrictions — is a
  tenant decision.
- **Access configuration.** Project roles, group memberships, marking
  grants, restricted view policies, application access, scoped session
  presets, OAuth application enablements, and service user role
  assignments are all configured by tenant administrators. The
  platform enforces what is configured; it cannot fix a misconfigured
  policy.
- **Monitoring.** The platform delivers audit events; the tenant's
  security team is responsible for routing them to a SIEM, building
  monitors, running access reviews, and acting on findings.
- **End-user behavior.** Phishing-resistant authentication factors,
  token hygiene, OAuth consent prompts, and the discipline to use
  scoped sessions and approved egress channels remain end-user
  responsibilities.

## Decision tree: "who do I call?"

| Symptom | Owner |
|---|---|
| A service is returning 5xx, audit events are not arriving, JWKS rotation fails | Platform operator |
| A user's organization, group, or attribute claim looks wrong after SSO | Tenant — IdP / SCIM source side, then enrollment admin |
| A user cannot access a project they should be able to | Project owner (role grant) or org admin (marking / group membership) |
| A user can access a resource they should not | Project owner + marking admin + audit reviewer; if the policy was correct and the platform leaked anyway, escalate to platform operator |
| An OAuth application is requesting unexpected scopes | Tenant — enrollment / org admin reviews the app registration & enablement |
| An egress policy is leaking data to an unintended destination | Tenant — egress policy approver; revoke the policy and review audit; loop platform operator if enforcement itself failed |
| A retention policy deleted data that should have been kept | Tenant — retention policy owner & approvals chain reviewed; platform operator confirms execution log integrity |
| Audit events are missing categories or fields | Platform operator (schema), then tenant security team for monitor coverage |

## When responsibilities shift

The line above is the **default**. Two scenarios shift it:

1. **Self-hosted deployments.** When the tenant *is* the operator, the
   entire operator column collapses onto the tenant's infra/security
   team. The
   [Protecting your self-hosted Foundry installation](https://www.palantir.com/docs/foundry/security/protect-foundry-installation)
   parity guidance, mirrored in
   [checklist `SG.54`](../migration/foundry-security-governance-1to1-checklist.md),
   becomes mandatory rather than advisory.
2. **Consumer mode.** When an organization is configured for
   consumer-style access (hidden user/group discovery, restricted app
   set, public registration flows), the tenant takes on additional
   responsibility for privacy boundaries. See
   [checklist `SG.42`](../migration/foundry-security-governance-1to1-checklist.md).

## Where to go next

- [Security overview](./security-overview.md) — the seven control
  layers and how they compose.
- [Identity and access](./identity-and-access.md) — IdP integration,
  users, groups, tokens, sessions.
- [Policies and authorization](./policies-and-authorization.md) —
  roles, ABAC, policy decision model.
- [Restricted views and data controls](./restricted-views-and-data-controls.md) —
  row/column-level data protection.
- [Audit and traceability](./audit-and-traceability.md) — what is
  recorded, where it goes, who can read it.
- [Foundry public-docs parity policy](../reference/foundry-public-docs-parity-policy.md) —
  scope boundary for any feature modeled on Foundry.

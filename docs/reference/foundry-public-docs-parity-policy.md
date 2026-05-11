# Foundry public-docs parity policy

Date: 2026-05-10  
Status: frozen scope boundary for Workstream `W0.3`

OpenFoundry may implement Foundry-style workflows using public information, but
it must not clone Palantir products, branding, private implementation details,
or proprietary assets. The target is **functional parity from public docs**:
users should be able to build equivalent workflows in OpenFoundry, with
OpenFoundry-owned code, design, resource models, tests, and documentation.

Primary public source:

- [Palantir Foundry documentation root](https://www.palantir.com/docs/foundry/)

## Parity boundary

| Area | Allowed | Not allowed |
| --- | --- | --- |
| Product behavior | Implement concepts, workflows, API semantics, and data models described in public documentation. | Copy private behavior discovered through tenant access, browser bundles, network traces, internal exports, or non-public training/materials. |
| Naming | Use familiar public concept names when useful: app, page, widget, variable, action, function, pipeline, transform, build, dataset output, object output. | Require Palantir private RID formats, hidden IDs, internal service names, or undocumented resource layouts. |
| User experience | Build an OpenFoundry-native UI that supports comparable tasks and interaction patterns. | Create a pixel-perfect Palantir UI clone, copy Palantir visual identity, or reuse proprietary screenshots as application assets. |
| Code | Write original implementation code and tests in this repository. | Copy, decompile, scrape, paste, or adapt Palantir source code, minified bundles, CSS, schemas from private tenants, or generated client code not published for reuse. |
| Assets | Use OpenFoundry-owned icons, public open-source assets with compatible licenses, or newly created assets. | Use Palantir logos, product marks, screenshots, icons, fonts, illustrations, or other proprietary assets in the product. |
| Documentation | Cite public Palantir docs when a feature is modeled after a documented concept. | Quote long passages from Palantir docs, reproduce complete pages, or import non-public manuals. |
| APIs | Provide OpenFoundry APIs that are concept-compatible where useful and documented locally. | Claim official Palantir API compatibility unless a specific public API behavior has been intentionally implemented and tested. |
| Branding | Mention Palantir/Foundry only in comparative planning docs and source citations. | Brand OpenFoundry UI, docs, demos, packages, images, or services as Palantir or Foundry products. |

## Allowed source types

- Public Palantir documentation pages.
- Public Palantir API reference pages.
- Public blog posts, videos, or talks, used only as high-level behavioral
  examples and cited when they influence a feature.
- User-supplied screenshots or notes when the user has the right to share them,
  used as workflow references, not copied as product assets.
- Open-source libraries and assets with licenses compatible with this repo.

## Disallowed source types

- Palantir source code, decompiled bundles, minified frontend assets, internal
  schemas, private API traces, tenant exports, or credentials.
- Non-public training decks, support materials, enablement docs, or customer
  tenant screenshots unless they are explicitly public and reusable.
- Palantir trademarks, logos, icons, fonts, screenshots, or proprietary design
  elements as OpenFoundry product assets.
- Text copied wholesale from Palantir docs beyond short cited snippets needed
  for commentary.

## Implementation rules

1. Cite the public documentation page that justifies each parity feature.
2. Prefer OpenFoundry-native APIs and storage unless compatibility requires a
   public Foundry-shaped field or concept.
3. Normalize compatibility aliases at service boundaries and persist canonical
   OpenFoundry names from the
   [Foundry compatibility glossary](./foundry-compatibility-glossary.md).
4. Treat screenshots as workflow hints. Recreate functionality with original UI
   structure, styling, components, and assets.
5. Keep Palantir/Foundry names out of user-facing runtime screens except where
   a docs page is explicitly comparing platforms.
6. If a feature depends on non-public behavior, mark the checklist item
   `blocked` and define an OpenFoundry-native behavior instead of guessing.

## Review checklist

Use this checklist for PRs that implement Pipeline Builder, Workshop,
Geospatial, Data Connection, Ontology Actions, or Functions parity.

- [ ] The PR cites public docs for each Foundry-inspired feature.
- [ ] The PR does not include Palantir source code, generated private clients,
      exported tenant schemas, browser bundles, or network traces.
- [ ] The PR does not add Palantir logos, icons, fonts, screenshots, or visual
      assets to OpenFoundry runtime surfaces.
- [ ] The UI is OpenFoundry-native and is not a pixel-perfect Palantir clone.
- [ ] New public names follow the
      [Foundry compatibility glossary](./foundry-compatibility-glossary.md).
- [ ] Compatibility aliases are decoded at API boundaries and normalized before
      persistence.
- [ ] Any unknown private behavior is documented as an assumption, local design
      decision, or blocked follow-up.
- [ ] Tests or docs evidence show the OpenFoundry behavior works independently
      of Palantir services.

## Escalation rule

When a contributor is unsure whether a source or asset is allowed, the default
answer is **do not use it**. Link the public docs that are available, describe
the missing behavior, and choose an OpenFoundry-native implementation that can
be tested locally.

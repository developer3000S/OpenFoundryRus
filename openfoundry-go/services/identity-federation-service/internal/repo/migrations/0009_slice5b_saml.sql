-- identity-federation-service slice 5b — SAML request state column.
--
-- Slice 5a's `oauth_state` row covers OIDC: state token (PK) +
-- PKCE verifier + nonce. Slice 5b adds SAML, which doesn't use
-- PKCE or OIDC nonces but does need to round-trip an
-- AuthnRequest.ID to validate the InResponseTo on the SAML
-- response (RFC 7522 §4.1.4.2).
--
-- We extend the existing table rather than create a sibling
-- because the lifecycle is identical (one-shot, TTL-bounded,
-- DELETE-on-consume) and the lookup pattern is the same:
-- `state = $1 AND expires_at > NOW()`. The OIDC path leaves
-- saml_request_id NULL; the SAML path leaves code_verifier +
-- nonce empty strings.
ALTER TABLE oauth_state
    ADD COLUMN IF NOT EXISTS saml_request_id TEXT;

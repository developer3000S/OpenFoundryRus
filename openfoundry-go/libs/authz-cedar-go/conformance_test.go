package cedarauthz_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	cedar "github.com/cedar-policy/cedar-go"
	"github.com/cedar-policy/cedar-go/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cedarauthz "github.com/openfoundry/openfoundry-go/libs/authz-cedar-go"
)

// These fixtures mirror the Rust authz-cedar policy/request/entity shape:
// one policy bundle, one Cedar request (principal/action/resource/context),
// and one entity graph per case. They intentionally exercise the same
// default-deny, permit-reason, and diagnostic-error contracts that Cedar's
// Rust Response exposes through diagnostics().reason()/errors().
func TestCedarConformancePolicyRequestEntityFixtures(t *testing.T) {
	t.Parallel()

	const clearancePolicy = `
		permit(
		  principal is User,
		  action == Action::"read",
		  resource is Dataset
		) when {
		  principal.tenant == resource.tenant &&
		  principal.clearances.containsAll(resource.markings)
		};
	`

	publicMark := types.NewEntityUID("Marking", "public")
	piiMark := types.NewEntityUID("Marking", "pii")
	readAction := types.NewEntityUID("Action", "read")
	writeAction := types.NewEntityUID("Action", "write")

	type fixture struct {
		name        string
		policies    []cedarauthz.PolicyRecord
		principal   cedar.EntityUID
		action      cedar.EntityUID
		resource    cedar.EntityUID
		context     cedar.Record
		entities    cedar.EntityMap
		want        cedar.Decision
		wantReasons []string
		wantErrors  []string
	}

	cases := []fixture{
		{
			name:      "allow when principal tenant and clearances cover resource markings",
			policies:  []cedarauthz.PolicyRecord{{ID: "permit-cleared-readers", Version: 1, Source: clearancePolicy}},
			principal: types.NewEntityUID("User", "alice"),
			action:    readAction,
			resource:  types.NewEntityUID("Dataset", "ds-public"),
			context: cedar.NewRecord(cedar.RecordMap{
				"request_id": cedar.String("req-allow-1"),
			}),
			entities: mkEntities(t,
				mkMarking(publicMark, "public"),
				mkUser(types.NewEntityUID("User", "alice"), "acme", []cedar.EntityUID{publicMark, piiMark}, []string{"reader"}),
				mkDataset(types.NewEntityUID("Dataset", "ds-public"), "ri.dataset.acme.ds-public", "acme", []cedar.EntityUID{publicMark}),
			),
			want:        cedar.Allow,
			wantReasons: []string{"permit-cleared-readers"},
		},
		{
			name:      "deny when action does not match the policy scope",
			policies:  []cedarauthz.PolicyRecord{{ID: "permit-cleared-readers", Version: 1, Source: clearancePolicy}},
			principal: types.NewEntityUID("User", "alice"),
			action:    writeAction,
			resource:  types.NewEntityUID("Dataset", "ds-public"),
			context: cedar.NewRecord(cedar.RecordMap{
				"request_id": cedar.String("req-deny-action"),
			}),
			entities: mkEntities(t,
				mkMarking(publicMark, "public"),
				mkUser(types.NewEntityUID("User", "alice"), "acme", []cedar.EntityUID{publicMark}, []string{"reader"}),
				mkDataset(types.NewEntityUID("Dataset", "ds-public"), "ri.dataset.acme.ds-public", "acme", []cedar.EntityUID{publicMark}),
			),
			want: cedar.Deny,
		},
		{
			name:      "deny when principal lacks one resource marking",
			policies:  []cedarauthz.PolicyRecord{{ID: "permit-cleared-readers", Version: 1, Source: clearancePolicy}},
			principal: types.NewEntityUID("User", "bob"),
			action:    readAction,
			resource:  types.NewEntityUID("Dataset", "ds-pii"),
			context: cedar.NewRecord(cedar.RecordMap{
				"request_id": cedar.String("req-deny-clearance"),
			}),
			entities: mkEntities(t,
				mkMarking(publicMark, "public"),
				mkMarking(piiMark, "pii"),
				mkUser(types.NewEntityUID("User", "bob"), "acme", []cedar.EntityUID{publicMark}, []string{"reader"}),
				mkDataset(types.NewEntityUID("Dataset", "ds-pii"), "ri.dataset.acme.ds-pii", "acme", []cedar.EntityUID{publicMark, piiMark}),
			),
			want: cedar.Deny,
		},
		{
			name:      "diagnostic error when fixture omits required principal attribute",
			policies:  []cedarauthz.PolicyRecord{{ID: "permit-cleared-readers", Version: 1, Source: clearancePolicy}},
			principal: types.NewEntityUID("User", "eve"),
			action:    readAction,
			resource:  types.NewEntityUID("Dataset", "ds-public"),
			context: cedar.NewRecord(cedar.RecordMap{
				"request_id": cedar.String("req-diagnostic-error"),
			}),
			entities: mkEntities(t,
				mkMarking(publicMark, "public"),
				cedar.Entity{
					UID: types.NewEntityUID("User", "eve"),
					Attributes: cedar.NewRecord(cedar.RecordMap{
						"tenant": cedar.String("acme"),
						"roles":  cedar.NewSet(cedar.String("reader")),
					}),
				},
				mkDataset(types.NewEntityUID("Dataset", "ds-public"), "ri.dataset.acme.ds-public", "acme", []cedar.EntityUID{publicMark}),
			),
			want:       cedar.Deny,
			wantErrors: []string{"permit-cleared-readers"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, err := cedarauthz.NewWithPolicies(tc.policies)
			require.NoError(t, err)
			eng := cedarauthz.NewEngineNoopAudit(store)

			out, err := eng.Authorize(context.Background(), tc.principal, tc.action, tc.resource, tc.context, tc.entities)
			require.NoError(t, err)
			assert.Equal(t, tc.want, out.Decision)
			assert.ElementsMatch(t, tc.wantReasons, out.PolicyIDs)
			for _, wantPolicyID := range tc.wantErrors {
				assert.Condition(t, func() bool {
					for _, msg := range out.Diagnostics {
						if strings.Contains(msg, wantPolicyID) && strings.Contains(msg, "does not have the attribute") {
							return true
						}
					}
					return false
				}, "diagnostics %q should include policy id %q and missing-attribute error", out.Diagnostics, wantPolicyID)
			}
		})
	}
}

func TestCedarConformancePermitForbidAndHierarchyFixtures(t *testing.T) {
	t.Parallel()

	// Adapted to the OpenFoundry schema from the AWS Cedar conformance shape:
	// a complete policy set, request tuple, and entity graph per fixture. This
	// covers Cedar's key evaluation invariants that the service depends on:
	// default deny, parent-graph membership, and forbid-overrides-permit.
	policies := []cedarauthz.PolicyRecord{
		{
			ID:      "permit-role-read-same-tenant",
			Version: 1,
			Source: `
				permit(
				  principal in Role::"reader",
				  action == Action::"read",
				  resource is Dataset
				) when {
				  principal.tenant == resource.tenant
				};
			`,
		},
		{
			ID:      "forbid-pii-without-break-glass",
			Version: 1,
			Source: `
				forbid(
				  principal,
				  action == Action::"read",
				  resource is Dataset
				) when {
				  resource.markings.contains(Marking::"pii") &&
				  !(principal in Role::"break_glass")
				};
			`,
		},
	}

	publicMark := types.NewEntityUID("Marking", "public")
	piiMark := types.NewEntityUID("Marking", "pii")
	readerRole := types.NewEntityUID("Role", "reader")
	breakGlassRole := types.NewEntityUID("Role", "break_glass")
	readAction := types.NewEntityUID("Action", "read")

	type fixture struct {
		name        string
		principal   cedar.EntityUID
		resource    cedar.EntityUID
		entities    cedar.EntityMap
		want        cedar.Decision
		wantReasons []string
	}

	cases := []fixture{
		{
			name:      "default deny when principal is not in reader role",
			principal: types.NewEntityUID("User", "no-role"),
			resource:  types.NewEntityUID("Dataset", "ds-public"),
			entities: mkEntities(t,
				mkMarking(publicMark, "public"),
				mkRole(readerRole, "reader"),
				mkUserWithParents(types.NewEntityUID("User", "no-role"), "acme", []cedar.EntityUID{publicMark}, nil),
				mkDataset(types.NewEntityUID("Dataset", "ds-public"), "ri.dataset.acme.ds-public", "acme", []cedar.EntityUID{publicMark}),
			),
			want: cedar.Deny,
		},
		{
			name:      "permit when role parent and tenant match",
			principal: types.NewEntityUID("User", "reader"),
			resource:  types.NewEntityUID("Dataset", "ds-public"),
			entities: mkEntities(t,
				mkMarking(publicMark, "public"),
				mkRole(readerRole, "reader"),
				mkUserWithParents(types.NewEntityUID("User", "reader"), "acme", []cedar.EntityUID{publicMark}, []cedar.EntityUID{readerRole}),
				mkDataset(types.NewEntityUID("Dataset", "ds-public"), "ri.dataset.acme.ds-public", "acme", []cedar.EntityUID{publicMark}),
			),
			want:        cedar.Allow,
			wantReasons: []string{"permit-role-read-same-tenant"},
		},
		{
			name:      "forbid overrides a matching permit for pii",
			principal: types.NewEntityUID("User", "reader-pii"),
			resource:  types.NewEntityUID("Dataset", "ds-pii"),
			entities: mkEntities(t,
				mkMarking(publicMark, "public"),
				mkMarking(piiMark, "pii"),
				mkRole(readerRole, "reader"),
				mkUserWithParents(types.NewEntityUID("User", "reader-pii"), "acme", []cedar.EntityUID{publicMark, piiMark}, []cedar.EntityUID{readerRole}),
				mkDataset(types.NewEntityUID("Dataset", "ds-pii"), "ri.dataset.acme.ds-pii", "acme", []cedar.EntityUID{publicMark, piiMark}),
			),
			want:        cedar.Deny,
			wantReasons: []string{"forbid-pii-without-break-glass"},
		},
		{
			name:      "break glass parent prevents forbid so permit can allow",
			principal: types.NewEntityUID("User", "reader-break-glass"),
			resource:  types.NewEntityUID("Dataset", "ds-pii"),
			entities: mkEntities(t,
				mkMarking(publicMark, "public"),
				mkMarking(piiMark, "pii"),
				mkRole(readerRole, "reader"),
				mkRole(breakGlassRole, "break_glass"),
				mkUserWithParents(types.NewEntityUID("User", "reader-break-glass"), "acme", []cedar.EntityUID{publicMark, piiMark}, []cedar.EntityUID{readerRole, breakGlassRole}),
				mkDataset(types.NewEntityUID("Dataset", "ds-pii"), "ri.dataset.acme.ds-pii", "acme", []cedar.EntityUID{publicMark, piiMark}),
			),
			want:        cedar.Allow,
			wantReasons: []string{"permit-role-read-same-tenant"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, err := cedarauthz.NewWithPolicies(policies)
			require.NoError(t, err)
			eng := cedarauthz.NewEngineNoopAudit(store)

			out, err := eng.Authorize(context.Background(), tc.principal, readAction, tc.resource, cedar.NewRecord(cedar.RecordMap{}), tc.entities)
			require.NoError(t, err)
			assert.Equal(t, tc.want, out.Decision)
			assert.ElementsMatch(t, tc.wantReasons, out.PolicyIDs)
		})
	}
}

func mkRole(uid cedar.EntityUID, id string) cedar.Entity {
	return cedar.Entity{
		UID: uid,
		Attributes: cedar.NewRecord(cedar.RecordMap{
			"id": cedar.String(id),
		}),
	}
}

func mkUserWithParents(uid cedar.EntityUID, tenant string, clearances []cedar.EntityUID, parents []cedar.EntityUID) cedar.Entity {
	user := mkUser(uid, tenant, clearances, nil)
	user.Parents = types.NewEntityUIDSet(parents...)
	return user
}

func TestCedarConformanceInvalidPolicyFixtureRejected(t *testing.T) {
	t.Parallel()
	store, err := cedarauthz.NewEmpty()
	require.NoError(t, err)

	err = store.ReplacePolicies([]cedarauthz.PolicyRecord{{
		ID:      "invalid-policy-fixture",
		Version: 1,
		Source:  `permit(principal, action == Action::"read", resource is Dataset) when { resource.not_declared == "x" };`,
	}})
	require.Error(t, err)
	var validation *cedarauthz.ValidationError
	assert.True(t, errors.As(err, &validation), "want ValidationError, got %T", err)
	assert.True(t, errors.Is(err, cedarauthz.ErrValidation))
	assert.Equal(t, 0, store.Len(), "invalid conformance fixture must not swap into the active store")
}

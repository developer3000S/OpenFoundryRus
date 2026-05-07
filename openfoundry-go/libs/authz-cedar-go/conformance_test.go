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

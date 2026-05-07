package adapters_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters/excel"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters/graphql"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters/ldap"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters/mssql"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters/oracle"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/adapters/sftp"
	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
)

// TestSkeletonStubsRegisterAndReportNotImplemented mirrors the CMA-14
// contract: the six placeholder connectors (Rust modules
// `connectors::{excel,graphql,ldap,mssql,oracle,sftp}.rs` are empty files)
// must (a) be registrable under their Rust module names, and (b) return
// [adapters.ErrNotImplemented] from every capability so the dispatcher in
// `internal/domain/discovery` translates the failure into the existing
// "<capability> is not supported for connector type: <name>" envelopes
// Rust emits.
func TestSkeletonStubsRegisterAndReportNotImplemented(t *testing.T) {
	cases := []struct {
		name    string
		factory adapters.Factory
	}{
		{excel.ConnectorType, excel.Factory()},
		{graphql.ConnectorType, graphql.Factory()},
		{ldap.ConnectorType, ldap.Factory()},
		{mssql.ConnectorType, mssql.Factory()},
		{oracle.ConnectorType, oracle.Factory()},
		{sftp.ConnectorType, sftp.Factory()},
	}

	r := adapters.NewRegistry()
	for _, tc := range cases {
		require.NoError(t, r.Register(tc.name, tc.factory), "register %s", tc.name)
	}
	require.Equal(t, []string{"excel", "graphql", "ldap", "mssql", "oracle", "sftp"}, r.Names())

	ctx := context.Background()
	conn := &models.Connection{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := r.Lookup(tc.name)
			require.NoError(t, err)

			_, err = a.DiscoverSources(ctx, conn, "")
			require.True(t, errors.Is(err, adapters.ErrNotImplemented), "DiscoverSources: %v", err)

			_, err = a.QueryVirtualTable(ctx, conn, &adapters.Query{}, "")
			require.True(t, errors.Is(err, adapters.ErrNotImplemented), "QueryVirtualTable: %v", err)

			_, err = a.StreamArrow(ctx, conn, &adapters.Query{}, "")
			require.True(t, errors.Is(err, adapters.ErrNotImplemented), "StreamArrow: %v", err)

			_, err = a.BuildIngestSpec(ctx, conn, &adapters.Source{})
			require.True(t, errors.Is(err, adapters.ErrNotImplemented), "BuildIngestSpec: %v", err)
		})
	}
}

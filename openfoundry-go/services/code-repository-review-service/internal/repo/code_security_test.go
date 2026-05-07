package repo

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodeSecurityRepoPersistsFindings(t *testing.T) {
	t.Parallel()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO code_security_scans")).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id", "payload", "created_at", "updated_at"}).
			AddRow(uuid.New(), []byte(`{"repository_rid":"ri.repo.1"}`), now, now))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO code_security_findings")).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnRows(pgxmock.NewRows([]string{"id", "parent_id", "payload", "created_at"}).
			AddRow(uuid.New(), uuid.New(), []byte(`{"rule_id":"fake.dynamic_eval"}`), now))
	mock.ExpectCommit()

	r := &CodeSecurityRepo{DB: mock}
	scan, findings, err := r.CreateScanWithFindings(context.Background(), json.RawMessage(`{"repository_rid":"ri.repo.1"}`), []json.RawMessage{json.RawMessage(`{"rule_id":"fake.dynamic_eval"}`)})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, scan.ID)
	require.Len(t, findings, 1)
	assert.JSONEq(t, `{"rule_id":"fake.dynamic_eval"}`, string(findings[0].Payload))
	require.NoError(t, mock.ExpectationsWereMet())
}

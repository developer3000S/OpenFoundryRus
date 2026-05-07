package models

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

type connectorContractFixture struct {
	Connection Connection `json:"connection"`
	SyncJob    SyncJob    `json:"sync_job"`
}

type wireModelsFixture struct {
	Agent                  ConnectorAgent         `json:"agent"`
	Credential             CredentialResponse     `json:"credential"`
	Registration           ConnectionRegistration `json:"registration"`
	VirtualTableSourceLink VirtualTableSourceLink `json:"virtual_table_source_link"`
	Capabilities           Capabilities           `json:"capabilities"`
	AutoRegisterRun        AutoRegisterRun        `json:"auto_register_run"`
	PollResult             PollResult             `json:"poll_result"`
	IcebergConfig          IcebergConfigResponse  `json:"iceberg_config"`
	WebhookResponse        InvokeWebhookResponse  `json:"webhook_response"`
	DevAuth                AuthenticatedResponse  `json:"dev_auth"`
	OutboxPayload          ConnectionChangedEvent `json:"outbox_payload"`
}

func TestConnectorManagementJSONContractFixture(t *testing.T) {
	assertFixtureRoundTrip(t, "testdata/connector_contract.json", &connectorContractFixture{})
}

func TestRustWireModelsFixture(t *testing.T) {
	assertFixtureRoundTrip(t, "testdata/wire_models.json", &wireModelsFixture{})
}

func TestEnumWireTokensMatchRustSerde(t *testing.T) {
	cases := []any{
		SourceProviderAmazonS3,
		TableTypeManagedIceberg,
		ComputePushdownEngineIbis,
		PollOutcomeChanged,
		SyncStatusRetrying,
		MediaSetSyncKindVirtual,
	}
	for _, value := range cases {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if len(encoded) < 3 || encoded[0] != '"' || encoded[len(encoded)-1] != '"' {
			t.Fatalf("enum did not marshal as JSON string: %s", encoded)
		}
	}
}
func assertFixtureRoundTrip(t *testing.T, path string, dst any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	encoded, err := json.Marshal(dst)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	assertSameJSON(t, raw, encoded)
}
func assertSameJSON(t *testing.T, wantRaw, gotRaw []byte) {
	t.Helper()
	var want, got any
	if err := json.Unmarshal(wantRaw, &want); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(gotRaw, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("JSON mismatch\nwant: %s\n got: %s", wantRaw, gotRaw)
	}
}

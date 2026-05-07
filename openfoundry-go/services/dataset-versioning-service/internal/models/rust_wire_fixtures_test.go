package models

import "testing"

type rustWireFixture struct {
	BranchCompare    BranchCompareResponse                 `json:"branch_compare"`
	BranchMarkings   BranchMarkingsView                    `json:"branch_markings"`
	BranchSource     BranchSource                          `json:"branch_source"`
	CatalogDataset   CatalogDataset                        `json:"catalog_dataset"`
	CatalogFacets    CatalogFacets                         `json:"catalog_facets"`
	DatasetHealth    DatasetHealth                         `json:"dataset_health"`
	DatasetModel     DatasetRichModel                      `json:"dataset_model"`
	FileListing      ListFilesOut                          `json:"file_listing"`
	MutationRequest  MutationRequest                       `json:"mutation_request"`
	Quality          DatasetQualityResponse                `json:"quality"`
	Retention        EffectiveRetention                    `json:"retention"`
	SchemaResponse   SchemaResponse                        `json:"schema_response"`
	SnapshotRequest  SnapshotRequest                       `json:"snapshot_request"`
	StorageDetails   StorageDetailsOut                     `json:"storage_details"`
	Transaction      RuntimeTransaction                    `json:"transaction"`
	TransactionBatch []BatchItemResult[RuntimeTransaction] `json:"transaction_batch"`
	UploadURL        UploadUrlOut                          `json:"upload_url"`
	ValidateResponse ValidateResponse                      `json:"validate_response"`
	View             ViewOut                               `json:"view"`
}

func TestRustWireJSONContractFixture(t *testing.T) {
	assertFixtureRoundTrip(t, "testdata/rust_wire_models.json", &rustWireFixture{})
}

func TestRustWireTokens(t *testing.T) {
	if TransactionTypeSnapshot != "SNAPSHOT" || TransactionTypeAppend != "APPEND" ||
		TransactionTypeUpdate != "UPDATE" || TransactionTypeDelete != "DELETE" {
		t.Fatalf("transaction type tokens drifted")
	}
	if TransactionStatusOpen != "OPEN" || TransactionStatusCommitted != "COMMITTED" || TransactionStatusAborted != "ABORTED" {
		t.Fatalf("transaction status tokens drifted")
	}
	if MarkingSourceParent != "PARENT" || MarkingSourceExplicit != "EXPLICIT" {
		t.Fatalf("marking source tokens drifted")
	}
	if FieldTypeStruct != "STRUCT" || FieldTypeArray != "ARRAY" || FieldTypeDecimal != "DECIMAL" {
		t.Fatalf("schema field discriminator tokens drifted")
	}
}

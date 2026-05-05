package audittrail

// AuditCategory enumerates the Foundry audit categories.
// JSON tokens are camelCase (`dataCreate`, `managementMarkings`, …) —
// matched verbatim with the Rust impl so SIEM rules stay portable.
type AuditCategory string

const (
	CategoryDataCreate         AuditCategory = "dataCreate"
	CategoryDataDelete         AuditCategory = "dataDelete"
	CategoryDataExport         AuditCategory = "dataExport"
	CategoryDataImport         AuditCategory = "dataImport"
	CategoryDataLoad           AuditCategory = "dataLoad"
	CategoryDataUpdate         AuditCategory = "dataUpdate"
	CategoryManagementMarkings AuditCategory = "managementMarkings"
)

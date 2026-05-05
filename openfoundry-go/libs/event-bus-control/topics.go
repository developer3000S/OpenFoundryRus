package controlbus

// Subject prefixes — kept verbatim with the Rust workspace so cross-
// language services route to the same hierarchy.
const (
	SubjectAuth           = "of.auth"
	SubjectDatasets       = "of.datasets"
	SubjectDatasetQuality = "of.datasets.quality"
	SubjectPipelines      = "of.pipelines"
	SubjectWorkflows      = "of.workflows"
	SubjectOntology       = "of.ontology"
	SubjectQueries        = "of.queries"
	SubjectAudit          = "of.audit"
	SubjectNotifications  = "of.notifications"
)

// JetStream stream names.
const (
	StreamEvents        = "OF_EVENTS"
	StreamAudit         = "OF_AUDIT"
	StreamNotifications = "OF_NOTIFICATIONS"
)

// Specific event-type / subject constants — match Rust contracts.rs.
const (
	DatasetQualityRefreshRequestedEventType = "dataset.quality.refresh.requested"
	DatasetQualityRefreshRequestedSubject   = "of.datasets.quality.refresh.requested"

	WorkflowTriggerRequestedEventType = "workflow.trigger.requested"
	WorkflowTriggerRequestedSubject   = "of.workflows.trigger.requested"

	NotificationEventType = "notification.updated"
	NotificationSubject   = "of.notifications.updated"
)

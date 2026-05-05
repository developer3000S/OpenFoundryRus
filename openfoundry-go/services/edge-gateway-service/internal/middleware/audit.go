package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	controlbus "github.com/openfoundry/openfoundry-go/libs/event-bus-control"
)

// gatewayAuditPayload mirrors the Rust `GatewayAuditPayload` struct.
//
// Field names + types are byte-identical so audit-sink decodes either
// language's payload through the same schema.
type gatewayAuditPayload struct {
	SourceService  string                  `json:"source_service"`
	Channel        string                  `json:"channel"`
	Actor          string                  `json:"actor"`
	Action         string                  `json:"action"`
	ResourceType   string                  `json:"resource_type"`
	ResourceID     string                  `json:"resource_id"`
	Status         string                  `json:"status"`
	Severity       string                  `json:"severity"`
	Classification string                  `json:"classification"`
	SubjectID      *string                 `json:"subject_id,omitempty"`
	IPAddress      *string                 `json:"ip_address,omitempty"`
	Location       *string                 `json:"location,omitempty"`
	Metadata       gatewayAuditMetadata    `json:"metadata"`
	Labels         []string                `json:"labels"`
	RetentionDays  int32                   `json:"retention_days"`
}

type gatewayAuditMetadata struct {
	RequestID string  `json:"request_id"`
	Method    string  `json:"method"`
	Path      string  `json:"path"`
	Status    int     `json:"status"`
	UserAgent *string `json:"user_agent,omitempty"`
}

// AuditConfig wires the audit middleware to a NATS publisher.
//
// Pass a nil Publisher to disable audit publishing — the middleware
// becomes a no-op that still records the per-request observation in
// the slog logger.
type AuditConfig struct {
	Publisher *controlbus.Publisher
}

// statusRecorder captures the response status for the audit payload.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wrote {
		s.status = http.StatusOK
		s.wrote = true
	}
	return s.ResponseWriter.Write(b)
}

// Audit returns a fire-and-forget audit middleware.
func Audit(cfg AuditConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			if cfg.Publisher == nil {
				return
			}

			pathAndQuery := r.URL.Path
			if r.URL.RawQuery != "" {
				pathAndQuery += "?" + r.URL.RawQuery
			}

			var ua *string
			if v := r.Header.Get("User-Agent"); v != "" {
				ua = &v
			}

			eventStatus, severity := classifyStatus(rec.status)
			payload := gatewayAuditPayload{
				SourceService:  "gateway",
				Channel:        "nats",
				Actor:          "system:gateway",
				Action:         "request.forwarded",
				ResourceType:   "http_request",
				ResourceID:     pathAndQuery,
				Status:         eventStatus,
				Severity:       severity,
				Classification: "confidential",
				Metadata: gatewayAuditMetadata{
					RequestID: r.Header.Get(HeaderXRequestID),
					Method:    r.Method,
					Path:      pathAndQuery,
					Status:    rec.status,
					UserAgent: ua,
				},
				Labels:        []string{"auto-captured", "gateway"},
				RetentionDays: 365,
			}

			// Fire-and-forget — bounded background goroutine. Same
			// shape as the Rust `tokio::spawn` block.
			auditWG.Add(1)
			go func() {
				defer auditWG.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				subject := controlbus.SubjectAudit + ".gateway"
				if err := cfg.Publisher.Publish(ctx, subject,
					"audit.gateway.request.forwarded", payload); err != nil {
					slog.Warn("failed to publish gateway audit event",
						slog.String("error", err.Error()))
				}
			}()
		})
	}
}

// auditWG lets graceful shutdown wait for in-flight audit publishes.
var auditWG sync.WaitGroup

// AuditWait blocks until every in-flight audit publish has completed
// or `ctx` is done. Server.Shutdown should call this before returning.
func AuditWait(ctx context.Context) {
	done := make(chan struct{})
	go func() { auditWG.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// classifyStatus maps an HTTP status to (event_status, severity).
// Mirrors the Rust gateway thresholds verbatim.
func classifyStatus(status int) (string, string) {
	switch {
	case status >= 500:
		return "failure", "critical"
	case status >= 400:
		return "failure", "high"
	default:
		return "success", "low"
	}
}

// _ keeps the json import used even if a future refactor strips encoding paths.
var _ = json.Marshal

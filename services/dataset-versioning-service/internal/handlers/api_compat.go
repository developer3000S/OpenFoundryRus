package handlers

import (
	"net/http"
	"strings"

	authmw "github.com/openfoundry/openfoundry-go/libs/auth-middleware"
)

const (
	apiErrorNotFound           = "NOT_FOUND"
	apiErrorPermissionDenied   = "PERMISSION_DENIED"
	apiErrorInvalidArgument    = "INVALID_ARGUMENT"
	apiErrorBranchNotFound     = "BRANCH_NOT_FOUND"
	apiErrorTransactionNotOpen = "TRANSACTION_NOT_OPEN"
	apiErrorSchemaParse        = "SCHEMA_PARSE_ERROR"
	apiErrorUnauthenticated    = "UNAUTHENTICATED"
	apiErrorConflict           = "CONFLICT"
	apiErrorUnavailable        = "UNAVAILABLE"
	apiErrorInternal           = "INTERNAL"

	datasetReadScope  = "datasets:read"
	datasetWriteScope = "datasets:write"
)

func writeCodedJSONErr(w http.ResponseWriter, status int, code string, msg string) {
	if code == "" {
		code = apiErrorCodeFor(status, msg)
	}
	writeJSON(w, status, map[string]string{
		"error":      msg,
		"message":    msg,
		"code":       code,
		"error_code": code,
	})
}

func writePermissionDenied(w http.ResponseWriter, requiredScope string, datasetRID string) {
	if requiredScope == "" {
		requiredScope = datasetReadScope
	}
	body := map[string]string{
		"error":                 "forbidden",
		"message":               "permission denied",
		"code":                  apiErrorPermissionDenied,
		"error_code":            apiErrorPermissionDenied,
		"required_scope":        requiredScope,
		"legacy_required_scope": legacyDatasetScope(requiredScope),
	}
	if datasetRID != "" {
		body["dataset_rid"] = datasetRID
	}
	writeJSON(w, http.StatusForbidden, body)
}

func legacyDatasetScope(scope string) string {
	switch scope {
	case datasetWriteScope:
		return "dataset.write"
	case datasetReadScope:
		return "dataset.read"
	default:
		return strings.ReplaceAll(scope, "datasets:", "dataset.")
	}
}

func writeTransactionNotOpen(w http.ResponseWriter) {
	writeJSON(w, http.StatusBadRequest, map[string]string{
		"error":      apiErrorTransactionNotOpen,
		"message":    "transaction is not OPEN",
		"code":       apiErrorTransactionNotOpen,
		"error_code": apiErrorTransactionNotOpen,
	})
}

func writeSchemaParseError(w http.ResponseWriter, msg string) {
	writeCodedJSONErr(w, http.StatusBadRequest, apiErrorSchemaParse, msg)
}

func apiErrorCodeFor(status int, msg string) string {
	lower := strings.ToLower(msg)
	switch {
	case status == http.StatusUnauthorized:
		return apiErrorUnauthenticated
	case status == http.StatusForbidden:
		return apiErrorPermissionDenied
	case status == http.StatusNotFound && strings.Contains(lower, "branch"):
		return apiErrorBranchNotFound
	case status == http.StatusNotFound:
		return apiErrorNotFound
	case status == http.StatusBadRequest && (strings.Contains(lower, "transaction_not_open") || strings.Contains(lower, "transaction is not open")):
		return apiErrorTransactionNotOpen
	case status == http.StatusBadRequest:
		return apiErrorInvalidArgument
	case status == http.StatusConflict:
		return apiErrorConflict
	case status == http.StatusServiceUnavailable:
		return apiErrorUnavailable
	default:
		if status >= 500 {
			return apiErrorInternal
		}
		return apiErrorInvalidArgument
	}
}

func canReadDataset(claims *authmw.Claims) bool {
	if claims == nil {
		return false
	}
	if claims.HasRole("admin") {
		return true
	}
	return hasDatasetScope(claims, "read") || hasDatasetScope(claims, "write") || hasDatasetScope(claims, "admin")
}

func hasDatasetScope(claims *authmw.Claims, action string) bool {
	keys := []string{
		"dataset:" + action,
		"datasets:" + action,
		"dataset." + action,
		"datasets." + action,
		"api:datasets-" + action,
		"api:datasets:" + action,
		"api.datasets." + action,
	}
	for _, key := range keys {
		if claims.HasPermissionKey(key) {
			return true
		}
	}
	return false
}

func DatasetAPIScopeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := authmw.FromContext(r.Context())
		if !ok {
			writeCodedJSONErr(w, http.StatusUnauthorized, apiErrorUnauthenticated, "authentication required")
			return
		}
		required := datasetReadScope
		if datasetAPIRequestMutates(r) {
			required = datasetWriteScope
		}
		if !claims.AllowsHTTPMethod(r.Method) || !claims.AllowsPath(r.URL.Path) {
			writePermissionDenied(w, required, datasetIDParam(r))
			return
		}
		if required == datasetWriteScope {
			if !canWriteDataset(claims) {
				writePermissionDenied(w, required, datasetIDParam(r))
				return
			}
		} else if !canReadDataset(claims) {
			writePermissionDenied(w, required, datasetIDParam(r))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func datasetAPIRequestMutates(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	case http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	case http.MethodPost:
		path := strings.TrimRight(r.URL.Path, "/")
		if strings.HasSuffix(path, "/datasets/getSchemaBatch") ||
			strings.HasSuffix(path, "/transactions:batchGet") ||
			strings.HasSuffix(path, "/schema:infer") {
			return false
		}
		return true
	default:
		return false
	}
}

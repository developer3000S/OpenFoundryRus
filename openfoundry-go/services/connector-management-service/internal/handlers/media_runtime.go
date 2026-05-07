package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/openfoundry/openfoundry-go/services/connector-management-service/internal/models"
)

type MediaSetRuntime interface {
	ExecuteMediaSetSync(ctx context.Context, sync *models.MediaSetSync, req *models.RunMediaSetSyncRequest, bearerToken string) (*models.MediaSetSyncExecutionReport, error)
}

type RuntimeErrorKind string

const (
	RuntimeUnavailable RuntimeErrorKind = "unavailable"
	RuntimeDispatch    RuntimeErrorKind = "dispatch_failed"
	RuntimeValidation  RuntimeErrorKind = "validation_failed"
)

type RuntimeError struct {
	Kind RuntimeErrorKind
	Msg  string
}

func (e *RuntimeError) Error() string { return e.Msg }

func runtimeErr(kind RuntimeErrorKind, msg string) error { return &RuntimeError{Kind: kind, Msg: msg} }

// HTTPMediaSetRuntime applies Foundry media-set sync filters over an already
// enumerated file batch and dispatches accepted files to media-sets-service.
type HTTPMediaSetRuntime struct {
	MediaSetsBaseURL string
	Client           *http.Client
}

func (rt *HTTPMediaSetRuntime) ExecuteMediaSetSync(ctx context.Context, sync *models.MediaSetSync, req *models.RunMediaSetSyncRequest, bearerToken string) (*models.MediaSetSyncExecutionReport, error) {
	if sync == nil || req == nil {
		return nil, runtimeErr(RuntimeValidation, "sync and request are required")
	}
	report, accepted, err := classifyMediaSetFiles(sync, req)
	if err != nil {
		return nil, runtimeErr(RuntimeValidation, err.Error())
	}
	base := strings.TrimRight(rt.MediaSetsBaseURL, "/")
	if base == "" {
		return nil, runtimeErr(RuntimeUnavailable, "media-sets-service URL is not configured")
	}
	client := rt.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	for _, file := range accepted {
		if err := dispatchMediaSetFile(ctx, client, base, sync, file, bearerToken); err != nil {
			report.DispatchErrors++
			return report, runtimeErr(RuntimeDispatch, err.Error())
		}
		report.Dispatched++
	}
	return report, nil
}

func classifyMediaSetFiles(sync *models.MediaSetSync, req *models.RunMediaSetSyncRequest) (*models.MediaSetSyncExecutionReport, []models.SourceFile, error) {
	already := map[string]struct{}{}
	for _, p := range req.AlreadySynced {
		already[p] = struct{}{}
	}
	allowed := map[string]struct{}{}
	for _, mt := range req.AllowedMIMETypes {
		allowed[strings.ToLower(mt)] = struct{}{}
	}
	report := &models.MediaSetSyncExecutionReport{SchemaMismatches: []string{}}
	accepted := []models.SourceFile{}
	for _, file := range req.SourceFiles {
		if strings.TrimSpace(file.Path) == "" {
			return nil, nil, fmt.Errorf("source file path must not be empty")
		}
		if sync.Filters.ExcludeAlreadySynced {
			if _, ok := already[file.Path]; ok {
				report.Stats.Skipped++
				continue
			}
		}
		if sync.Filters.FileSizeLimit != nil && file.SizeBytes > *sync.Filters.FileSizeLimit {
			report.Stats.Skipped++
			continue
		}
		if sync.Filters.PathGlob != nil {
			matched, err := matchMediaGlob(*sync.Filters.PathGlob, file.Path)
			if err != nil {
				return nil, nil, err
			}
			if !matched {
				report.Stats.Skipped++
				continue
			}
		}
		if len(allowed) > 0 {
			if _, ok := allowed[strings.ToLower(file.MimeType)]; !ok {
				if sync.Filters.IgnoreUnmatchedSchema {
					report.Stats.Skipped++
				} else {
					report.Stats.SchemaMismatched++
					report.SchemaMismatches = append(report.SchemaMismatches, file.Path)
				}
				continue
			}
		}
		report.Stats.Accepted++
		accepted = append(accepted, file)
	}
	return report, accepted, nil
}

func matchMediaGlob(pattern, name string) (bool, error) {
	matched, err := filepath.Match(pattern, name)
	if err != nil || matched {
		return matched, err
	}
	if strings.HasPrefix(pattern, "**/") {
		return filepath.Match(strings.TrimPrefix(pattern, "**/"), name)
	}
	return false, nil
}

func dispatchMediaSetFile(ctx context.Context, client *http.Client, base string, sync *models.MediaSetSync, file models.SourceFile, bearerToken string) error {
	var url string
	var body map[string]any
	if sync.Kind == models.MediaSetSyncKindVirtual {
		url = fmt.Sprintf("%s/media-sets/%s/virtual-items", base, sync.TargetMediaSetRID)
		physical := strings.Trim(strings.TrimRight(sync.Subfolder, "/")+"/"+strings.TrimLeft(file.Path, "/"), "/")
		body = map[string]any{"physical_path": physical, "item_path": file.Path, "mime_type": file.MimeType, "size_bytes": file.SizeBytes}
	} else {
		url = fmt.Sprintf("%s/media-sets/%s/items/upload-url", base, sync.TargetMediaSetRID)
		body = map[string]any{"path": file.Path, "mime_type": file.MimeType, "size_bytes": file.SizeBytes}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if bearerToken != "" {
		httpReq.Header.Set("Authorization", bearerToken)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("media-sets-service returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func runtimeHTTPStatus(err error) int {
	var rt *RuntimeError
	if !errors.As(err, &rt) {
		return http.StatusInternalServerError
	}
	switch rt.Kind {
	case RuntimeValidation:
		return http.StatusBadRequest
	case RuntimeUnavailable:
		return http.StatusServiceUnavailable
	case RuntimeDispatch:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

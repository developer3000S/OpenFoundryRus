package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type smokeSuite struct {
	BaseURL        string            `json:"base_url"`
	DefaultHeaders map[string]string `json:"default_headers"`
	Variables      map[string]any    `json:"variables"`
	Auth           map[string]any    `json:"auth"`
	Steps          []smokeStep       `json:"steps"`
}

type smokeStep struct {
	Name           string            `json:"name"`
	Method         string            `json:"method"`
	Path           string            `json:"path"`
	Headers        map[string]string `json:"headers"`
	Body           any               `json:"body"`
	ExpectedStatus int               `json:"expected_status"`
	Capture        map[string]string `json:"capture"`
	Expect         []smokeExpect     `json:"expect"`
	Retries        int               `json:"retries"`
}

type smokeExpect struct {
	Path   string `json:"path"`
	Equals any    `json:"equals"`
	Exists bool   `json:"exists"`
}
type smokeReport struct {
	BaseURL          string            `json:"base_url"`
	StartedAtEpochMS int64             `json:"started_at_epoch_ms"`
	Success          bool              `json:"success"`
	FailureMessage   *string           `json:"failure_message"`
	Steps            []smokeStepReport `json:"steps"`
}
type smokeStepReport struct {
	Name, Method, Path, URL string
	ExpectedStatus          int              `json:"expected_status"`
	ActualStatus            *int             `json:"actual_status"`
	Assertions              []smokeAssertion `json:"assertions"`
	Captured                map[string]any   `json:"captured"`
	ResponsePreview         *string          `json:"response_preview"`
}
type smokeAssertion struct {
	Path, Status string
	Expected     any `json:"expected,omitempty"`
	Actual       any `json:"actual,omitempty"`
}

func runSmokeSuite(ctx context.Context, scenarioPath, outputPath string) error {
	var suite smokeSuite
	if err := readScenario(scenarioPath, &suite); err != nil {
		return err
	}
	vars := map[string]any{"RUN_ID": uuid.NewString(), "STARTED_AT_EPOCH_MS": time.Now().UnixMilli()}
	for k, v := range suite.Variables {
		vars[k] = v
	}
	client := &http.Client{Timeout: 60 * time.Second}
	report := smokeReport{BaseURL: suite.BaseURL, StartedAtEpochMS: time.Now().UnixMilli(), Success: true}
	base := strings.TrimRight(suite.BaseURL, "/")
	for _, step := range suite.Steps {
		if step.ExpectedStatus == 0 {
			step.ExpectedStatus = http.StatusOK
		}
		path, err := resolveString(step.Path, vars)
		if err != nil {
			return writeSmokeFailure(outputPath, &report, fmt.Errorf("step %q failed before request: %w", step.Name, err))
		}
		url := base + "/" + strings.TrimLeft(path, "/")
		stepReport, err := executeSmokeStep(ctx, client, suite, step, url, vars)
		report.Steps = append(report.Steps, stepReport)
		if err != nil {
			return writeSmokeFailure(outputPath, &report, err)
		}
	}
	return writeJSON(outputPath, report)
}

func executeSmokeStep(ctx context.Context, client *http.Client, suite smokeSuite, step smokeStep, url string, vars map[string]any) (smokeStepReport, error) {
	var last smokeStepReport
	attempts := step.Retries + 1
	for attempt := 0; attempt < attempts; attempt++ {
		body, err := resolveJSON(step.Body, vars)
		if err != nil {
			return last, err
		}
		var reader io.Reader
		if body != nil {
			data, _ := json.Marshal(body)
			reader = bytes.NewReader(data)
		}
		req, err := http.NewRequestWithContext(ctx, step.Method, url, reader)
		if err != nil {
			return last, err
		}
		for k, v := range suite.DefaultHeaders {
			req.Header.Set(k, v)
		}
		for k, v := range step.Headers {
			vv, err := resolveString(v, vars)
			if err != nil {
				return last, err
			}
			req.Header.Set(k, vv)
		}
		if body != nil && req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/json")
		}
		applySmokeAuth(req, suite.Auth, vars)
		resp, err := client.Do(req)
		if err != nil {
			last = newSmokeStepReport(step, url, nil, nil, nil)
			continue
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		status := resp.StatusCode
		preview := truncate(string(data), 2048)
		var decoded any
		_ = json.Unmarshal(data, &decoded)
		captured := map[string]any{}
		for key, path := range step.Capture {
			if value, ok := valueAt(decoded, path); ok {
				vars[key] = value
				captured[key] = value
			}
		}
		assertions, assertOK := evaluateExpectations(decoded, step.Expect)
		last = newSmokeStepReport(step, url, &status, &preview, captured)
		last.Assertions = assertions
		if status == step.ExpectedStatus && assertOK {
			return last, nil
		}
	}
	return last, fmt.Errorf("step %q failed: expected status %d got %v", step.Name, step.ExpectedStatus, last.ActualStatus)
}

func newSmokeStepReport(step smokeStep, url string, status *int, preview *string, captured map[string]any) smokeStepReport {
	if captured == nil {
		captured = map[string]any{}
	}
	return smokeStepReport{Name: step.Name, Method: step.Method, Path: step.Path, URL: url, ExpectedStatus: step.ExpectedStatus, ActualStatus: status, Captured: captured, ResponsePreview: preview}
}
func writeSmokeFailure(path string, report *smokeReport, err error) error {
	msg := err.Error()
	report.Success = false
	report.FailureMessage = &msg
	_ = writeJSON(path, report)
	return err
}

func applySmokeAuth(req *http.Request, auth map[string]any, vars map[string]any) {
	if len(auth) == 0 {
		return
	}
	if token, ok := stringMap(auth, "bearer_token"); ok {
		req.Header.Set("Authorization", "Bearer "+token)
		return
	}
	if env, ok := stringMap(auth, "bearer_token_env"); ok {
		if token := os.Getenv(env); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		return
	}
	if basic, ok := auth["basic"].(map[string]any); ok {
		user, _ := basic["username"].(string)
		pass, _ := basic["password"].(string)
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+pass)))
	}
	if _, ok := auth["smoke_jwt"].(map[string]any); ok {
		req.Header.Set("Authorization", "Bearer smoke."+fmt.Sprint(vars["RUN_ID"])+".token")
	}
}

func evaluateExpectations(decoded any, expects []smokeExpect) ([]smokeAssertion, bool) {
	okAll := true
	out := make([]smokeAssertion, 0, len(expects))
	for _, exp := range expects {
		actual, ok := valueAt(decoded, exp.Path)
		assertion := smokeAssertion{Path: exp.Path, Status: "passed", Expected: exp.Equals, Actual: actual}
		if exp.Exists && !ok {
			assertion.Status = "failed"
			okAll = false
		}
		if exp.Equals != nil && !jsonEqual(exp.Equals, actual) {
			assertion.Status = "failed"
			okAll = false
		}
		out = append(out, assertion)
	}
	return out, okAll
}

// Benchmark

type benchSuite struct {
	BaseURL           string            `json:"base_url"`
	DefaultHeaders    map[string]string `json:"default_headers"`
	WarmupIterations  int               `json:"warmup_iterations"`
	MeasureIterations int               `json:"measure_iterations"`
	Scenarios         []benchScenario   `json:"scenarios"`
}
type benchScenario struct {
	Name, Method, Path string
	Headers            map[string]string `json:"headers"`
	Body               any               `json:"body"`
	ExpectedStatus     int               `json:"expected_status"`
	Tags               []string          `json:"tags"`
}
type benchReport struct {
	BaseURL           string                `json:"base_url"`
	StartedAtEpochMS  int64                 `json:"started_at_epoch_ms"`
	WarmupIterations  int                   `json:"warmup_iterations"`
	MeasureIterations int                   `json:"measure_iterations"`
	Scenarios         []benchScenarioReport `json:"scenarios"`
}
type benchScenarioReport struct {
	Name, Method, Path string
	ExpectedStatus     int      `json:"expected_status"`
	Statuses           []int    `json:"statuses"`
	Iterations         int      `json:"iterations"`
	MeanLatencyMS      float64  `json:"mean_latency_ms"`
	P95LatencyMS       float64  `json:"p95_latency_ms"`
	FastestLatencyMS   float64  `json:"fastest_latency_ms"`
	SlowestLatencyMS   float64  `json:"slowest_latency_ms"`
	Tags               []string `json:"tags"`
}

func runBenchmarkSuite(ctx context.Context, scenarioPath, outputPath string) error {
	var suite benchSuite
	if err := readScenario(scenarioPath, &suite); err != nil {
		return err
	}
	if suite.WarmupIterations == 0 {
		suite.WarmupIterations = 1
	}
	if suite.MeasureIterations == 0 {
		suite.MeasureIterations = 5
	}
	client := &http.Client{Timeout: 60 * time.Second}
	report := benchReport{BaseURL: suite.BaseURL, StartedAtEpochMS: time.Now().UnixMilli(), WarmupIterations: suite.WarmupIterations, MeasureIterations: suite.MeasureIterations}
	var mismatches []string
	for _, sc := range suite.Scenarios {
		for i := 0; i < suite.WarmupIterations; i++ {
			_, _ = executeBench(ctx, client, suite, sc)
		}
		var statuses []int
		var latencies []float64
		for i := 0; i < suite.MeasureIterations; i++ {
			start := time.Now()
			status, err := executeBench(ctx, client, suite, sc)
			if err != nil {
				return err
			}
			statuses = append(statuses, status)
			latencies = append(latencies, float64(time.Since(start).Microseconds())/1000.0)
		}
		for _, st := range statuses {
			if st != sc.ExpectedStatus {
				mismatches = append(mismatches, fmt.Sprintf("%s returned %v but expected %d", sc.Name, statuses, sc.ExpectedStatus))
				break
			}
		}
		report.Scenarios = append(report.Scenarios, benchScenarioReport{Name: sc.Name, Method: sc.Method, Path: sc.Path, ExpectedStatus: sc.ExpectedStatus, Statuses: statuses, Iterations: len(latencies), MeanLatencyMS: mean(latencies), P95LatencyMS: percentile(latencies, .95), FastestLatencyMS: min(latencies), SlowestLatencyMS: max(latencies), Tags: sc.Tags})
	}
	if err := writeJSON(outputPath, report); err != nil {
		return err
	}
	if len(mismatches) > 0 {
		return errors.New(strings.Join(mismatches, "; "))
	}
	return nil
}

func executeBench(ctx context.Context, client *http.Client, suite benchSuite, sc benchScenario) (int, error) {
	var reader io.Reader
	if sc.Body != nil {
		data, _ := json.Marshal(sc.Body)
		reader = bytes.NewReader(data)
	}
	url := strings.TrimRight(suite.BaseURL, "/") + "/" + strings.TrimLeft(sc.Path, "/")
	req, err := http.NewRequestWithContext(ctx, sc.Method, url, reader)
	if err != nil {
		return 0, err
	}
	for k, v := range suite.DefaultHeaders {
		req.Header.Set(k, v)
	}
	for k, v := range sc.Headers {
		req.Header.Set(k, v)
	}
	if sc.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func readScenario(path string, dest any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	resolved, err := resolveEnvTemplates(string(raw))
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(resolved), dest)
}
func resolveEnvTemplates(raw string) (string, error) {
	re := regexp.MustCompile(`\$\{([A-Z0-9_]+)\}`)
	missing := map[string]bool{}
	out := re.ReplaceAllStringFunc(raw, func(s string) string {
		key := re.FindStringSubmatch(s)[1]
		v, ok := os.LookupEnv(key)
		if !ok {
			missing[key] = true
			return s
		}
		return v
	})
	if len(missing) > 0 {
		keys := make([]string, 0, len(missing))
		for k := range missing {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return "", fmt.Errorf("missing environment variables: %s", strings.Join(keys, ", "))
	}
	return out, nil
}
func resolveString(raw string, vars map[string]any) (string, error) {
	re := regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)
	missing := map[string]bool{}
	out := re.ReplaceAllStringFunc(raw, func(s string) string {
		key := re.FindStringSubmatch(s)[1]
		v, ok := vars[key]
		if !ok {
			missing[key] = true
			return s
		}
		return fmt.Sprint(v)
	})
	if len(missing) > 0 {
		keys := make([]string, 0, len(missing))
		for k := range missing {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return "", fmt.Errorf("missing smoke variables: %s", strings.Join(keys, ", "))
	}
	return out, nil
}
func resolveJSON(value any, vars map[string]any) (any, error) {
	switch v := value.(type) {
	case string:
		if key, ok := exactTemplate(v); ok {
			val, exists := vars[key]
			if !exists {
				return nil, fmt.Errorf("missing smoke variable: %s", key)
			}
			return val, nil
		}
		return resolveString(v, vars)
	case []any:
		out := make([]any, len(v))
		for i := range v {
			r, err := resolveJSON(v[i], vars)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	case map[string]any:
		out := map[string]any{}
		for k, item := range v {
			r, err := resolveJSON(item, vars)
			if err != nil {
				return nil, err
			}
			out[k] = r
		}
		return out, nil
	default:
		return v, nil
	}
}
func exactTemplate(raw string) (string, bool) {
	re := regexp.MustCompile(`^\$\{([A-Za-z0-9_]+)\}$`)
	m := re.FindStringSubmatch(raw)
	if len(m) == 2 {
		return m[1], true
	}
	return "", false
}
func valueAt(value any, path string) (any, bool) {
	if path == "" || path == "." || path == "$" {
		return value, true
	}
	cur := value
	for _, part := range strings.Split(strings.Trim(strings.TrimPrefix(path, "$"), "."), ".") {
		if part == "" {
			continue
		}
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}
func jsonEqual(a, b any) bool {
	aa, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return bytes.Equal(aa, bb)
}
func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFile(path, data)
}
func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit]
}
func stringMap(m map[string]any, key string) (string, bool) { v, ok := m[key].(string); return v, ok }
func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}
func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	cp := append([]float64(nil), xs...)
	sort.Float64s(cp)
	idx := int(math.Ceil(float64(len(cp))*p)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	return cp[idx]
}
func min(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	m := xs[0]
	for _, x := range xs {
		if x < m {
			m = x
		}
	}
	return m
}
func max(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	m := xs[0]
	for _, x := range xs {
		if x > m {
			m = x
		}
	}
	return m
}

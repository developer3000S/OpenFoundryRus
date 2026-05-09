// Package spark renders SparkApplication custom resources and submits them via
// a Kubernetes dynamic HTTP client. It mirrors the Rust kube-rs surface while
// keeping the handler testable behind SparkClient.
package spark

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	SparkGroup         = "sparkoperator.k8s.io"
	SparkVersion       = "v1beta2"
	SparkKind          = "SparkApplication"
	SparkPlural        = "sparkapplications"
	MaxSparkAppNameLen = 50
)

const pipelineRunTemplate = `apiVersion: sparkoperator.k8s.io/v1beta2
kind: SparkApplication
metadata:
  name: ${spark_app_name}
  namespace: ${namespace}
spec:
  type: ${spark_application_type}
  mode: cluster
  image: ${pipeline_runner_image}
  imagePullPolicy: IfNotPresent
  mainClass: "${main_class}"
  mainApplicationFile: ${main_application_file}
  arguments:
    - "--pipeline-id"
    - "${pipeline_id}"
    - "--run-id"
    - "${run_id}"
    - "--input-dataset"
    - "${input_dataset_rid}"
    - "--output-dataset"
    - "${output_dataset_rid}"
    - "--catalog"
    - "${catalog}"
    - "--catalog-uri"
    - "${catalog_uri}"
    - "--inline-sql"
    - "${inline_sql}"
    - "--inline-format"
    - "${inline_format}"
  sparkVersion: "3.5.4"
  restartPolicy:
    type: Never
  driver:
    cores: ${driver_cores}
    memory: ${driver_memory}
    serviceAccount: spark
    env:
      - name: AWS_ACCESS_KEY_ID
        valueFrom:
          secretKeyRef:
            name: openfoundry-iceberg
            key: AWS_ACCESS_KEY_ID
            optional: true
      - name: AWS_SECRET_ACCESS_KEY
        valueFrom:
          secretKeyRef:
            name: openfoundry-iceberg
            key: AWS_SECRET_ACCESS_KEY
            optional: true
  executor:
    cores: ${executor_cores}
    instances: ${executor_instances}
    memory: ${executor_memory}
    env:
      - name: AWS_ACCESS_KEY_ID
        valueFrom:
          secretKeyRef:
            name: openfoundry-iceberg
            key: AWS_ACCESS_KEY_ID
            optional: true
      - name: AWS_SECRET_ACCESS_KEY
        valueFrom:
          secretKeyRef:
            name: openfoundry-iceberg
            key: AWS_SECRET_ACCESS_KEY
            optional: true
  sparkConf:
    spark.sql.extensions: "org.apache.iceberg.spark.extensions.IcebergSparkSessionExtensions"
    spark.sql.catalog.${catalog}: "org.apache.iceberg.spark.SparkCatalog"
    spark.sql.catalog.${catalog}.type: "rest"
    spark.sql.catalog.${catalog}.uri: "${catalog_uri}"
    spark.hadoop.fs.s3a.endpoint: "${s3_endpoint}"
    spark.hadoop.fs.s3a.path.style.access: "true"
    spark.hadoop.fs.s3a.aws.credentials.provider: "org.apache.hadoop.fs.s3a.SimpleAWSCredentialsProvider"
`

type SparkApplicationType string

const (
	SparkApplicationScala  SparkApplicationType = "Scala"
	SparkApplicationPython SparkApplicationType = "Python"
)

func (t SparkApplicationType) defaults() (string, string) {
	switch t {
	case SparkApplicationPython:
		return "", "local:///opt/spark/work-dir/pipeline_runner.py"
	default:
		return "com.openfoundry.pipeline.PipelineRunner", "local:///opt/spark/jars/pipeline-runner-spark.jar"
	}
}

type SparkResourceOverrides struct {
	DriverCores       uint32 `json:"driver_cores"`
	DriverMemory      string `json:"driver_memory"`
	ExecutorCores     uint32 `json:"executor_cores"`
	ExecutorInstances uint32 `json:"executor_instances"`
	ExecutorMemory    string `json:"executor_memory"`
}

func DefaultSparkResourceOverrides() SparkResourceOverrides {
	return SparkResourceOverrides{DriverCores: 1, DriverMemory: "1g", ExecutorCores: 1, ExecutorInstances: 2, ExecutorMemory: "2g"}
}

func (r SparkResourceOverrides) withDefaults() SparkResourceOverrides {
	d := DefaultSparkResourceOverrides()
	if r.DriverCores != 0 {
		d.DriverCores = r.DriverCores
	}
	if strings.TrimSpace(r.DriverMemory) != "" {
		d.DriverMemory = r.DriverMemory
	}
	if r.ExecutorCores != 0 {
		d.ExecutorCores = r.ExecutorCores
	}
	if r.ExecutorInstances != 0 {
		d.ExecutorInstances = r.ExecutorInstances
	}
	if strings.TrimSpace(r.ExecutorMemory) != "" {
		d.ExecutorMemory = r.ExecutorMemory
	}
	return d
}

type PipelineRunInput struct {
	PipelineID          string                 `json:"pipeline_id"`
	RunID               string                 `json:"run_id"`
	Namespace           string                 `json:"namespace"`
	ApplicationType     SparkApplicationType   `json:"application_type"`
	PipelineRunnerImage string                 `json:"pipeline_runner_image"`
	InputDatasetRID     string                 `json:"input_dataset_rid"`
	OutputDatasetRID    string                 `json:"output_dataset_rid"`
	Resources           SparkResourceOverrides `json:"resources"`
	// New fields for inline SQL transforms (consumed by services/pipeline-runner-spark
	// PipelineRunner.scala via spark-submit). See infra/helm/infra/spark-jobs/templates/_pipeline-run-template.yaml.
	Catalog      string `json:"catalog,omitempty"`
	CatalogURI   string `json:"catalog_uri,omitempty"`
	S3Endpoint   string `json:"s3_endpoint,omitempty"`
	InlineSQL    string `json:"inline_sql,omitempty"`
	InlineFormat string `json:"inline_format,omitempty"`
}

// PipelineRunDefaults applies sensible defaults for the inline-SQL fields when
// the caller leaves them blank. Centralised so the engine wiring and the API
// handlers stay in sync.
func PipelineRunDefaults(input PipelineRunInput) PipelineRunInput {
	if strings.TrimSpace(input.Catalog) == "" {
		input.Catalog = "lakekeeper"
	}
	if strings.TrimSpace(input.CatalogURI) == "" {
		input.CatalogURI = "http://lakekeeper.lakekeeper.svc:8181"
	}
	if strings.TrimSpace(input.S3Endpoint) == "" {
		input.S3Endpoint = "http://rook-ceph-rgw-openfoundry-store.rook-ceph.svc:80"
	}
	if strings.TrimSpace(input.InlineFormat) == "" {
		input.InlineFormat = "iceberg"
	}
	return input
}

type SparkRunStatus string

const (
	SparkRunSubmitted SparkRunStatus = "SUBMITTED"
	SparkRunRunning   SparkRunStatus = "RUNNING"
	SparkRunSucceeded SparkRunStatus = "SUCCEEDED"
	SparkRunFailed    SparkRunStatus = "FAILED"
	SparkRunUnknown   SparkRunStatus = "UNKNOWN"
)

type SparkRunStatusReport struct {
	Status       SparkRunStatus `json:"status"`
	ErrorMessage *string        `json:"error_message,omitempty"`
}

type SparkClient interface {
	SubmitPipelineRun(ctx context.Context, input PipelineRunInput) (string, error)
	GetPipelineRunStatus(ctx context.Context, namespace, name string) (*SparkRunStatusReport, error)
}

type InvalidInputError struct{ Message string }

func (e *InvalidInputError) Error() string { return "invalid pipeline run input: " + e.Message }

type RenderError struct{ Message string }

func (e *RenderError) Error() string { return "template rendering failed: " + e.Message }

type KubeError struct {
	StatusCode int
	Message    string
}

func (e *KubeError) Error() string { return fmt.Sprintf("kubernetes api call failed: %s", e.Message) }

type UnavailableError struct{ Err error }

func (e *UnavailableError) Error() string { return "kubernetes client not configured" }
func (e *UnavailableError) Unwrap() error { return e.Err }

func RenderManifest(input PipelineRunInput) (map[string]any, error) {
	if input.ApplicationType == "" {
		input.ApplicationType = SparkApplicationScala
	}
	input.Resources = input.Resources.withDefaults()
	input = PipelineRunDefaults(input)
	if err := validateInput(input); err != nil {
		return nil, err
	}
	name, err := SparkAppName(input.PipelineID, input.RunID)
	if err != nil {
		return nil, err
	}
	mainClass, mainFile := input.ApplicationType.defaults()
	vars := map[string]string{
		"spark_app_name":         name,
		"pipeline_id":            input.PipelineID,
		"run_id":                 input.RunID,
		"namespace":              input.Namespace,
		"spark_application_type": string(input.ApplicationType),
		"pipeline_runner_image":  input.PipelineRunnerImage,
		"main_class":             mainClass,
		"main_application_file":  mainFile,
		"input_dataset_rid":      input.InputDatasetRID,
		"output_dataset_rid":     input.OutputDatasetRID,
		"catalog":                input.Catalog,
		"catalog_uri":            input.CatalogURI,
		"s3_endpoint":            input.S3Endpoint,
		"inline_sql":             escapeYAMLString(input.InlineSQL),
		"inline_format":          input.InlineFormat,
		"driver_cores":           fmt.Sprint(input.Resources.DriverCores),
		"driver_memory":          input.Resources.DriverMemory,
		"executor_cores":         fmt.Sprint(input.Resources.ExecutorCores),
		"executor_instances":     fmt.Sprint(input.Resources.ExecutorInstances),
		"executor_memory":        input.Resources.ExecutorMemory,
	}
	rendered := pipelineRunTemplate
	for key, value := range vars {
		rendered = strings.ReplaceAll(rendered, "${"+key+"}", value)
	}
	if missing := firstUnsubstitutedPlaceholder(rendered); missing != "" {
		return nil, &RenderError{Message: "unresolved placeholder ${" + missing + "} in pipeline-run template"}
	}
	var yamlObj any
	if err := yaml.Unmarshal([]byte(rendered), &yamlObj); err != nil {
		return nil, &RenderError{Message: "YAML parse: " + err.Error()}
	}
	jsonObj := normalizeYAML(yamlObj).(map[string]any)
	meta, _ := jsonObj["metadata"].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
		jsonObj["metadata"] = meta
	}
	meta["name"] = name
	meta["namespace"] = input.Namespace
	return jsonObj, nil
}

func SparkAppName(pipelineID, runID string) (string, error) {
	name := fmt.Sprintf("pipeline-run-%s-%s", pipelineID, runID)
	if len(name) > MaxSparkAppNameLen {
		return "", &InvalidInputError{Message: fmt.Sprintf("computed SparkApplication name %q (%d chars) exceeds the %d-char limit; truncate pipeline_id / run_id before submission", name, len(name), MaxSparkAppNameLen)}
	}
	return name, nil
}

func validateInput(input PipelineRunInput) error {
	for label, value := range map[string]string{"pipeline_id": input.PipelineID, "run_id": input.RunID, "namespace": input.Namespace, "pipeline_runner_image": input.PipelineRunnerImage, "input_dataset_rid": input.InputDatasetRID, "output_dataset_rid": input.OutputDatasetRID} {
		if strings.TrimSpace(value) == "" {
			return &InvalidInputError{Message: label + " must not be empty"}
		}
	}
	return nil
}

type KubernetesClient struct {
	BaseURL     string
	BearerToken string
	HTTPClient  *http.Client
}

func NewKubernetesClient(baseURL, bearerToken string, httpClient *http.Client) (*KubernetesClient, error) {
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, err
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &KubernetesClient{BaseURL: strings.TrimRight(baseURL, "/"), BearerToken: bearerToken, HTTPClient: httpClient}, nil
}

func NewKubernetesClientFromEnv() (*KubernetesClient, error) {
	if explicit := os.Getenv("KUBERNETES_API_URL"); explicit != "" {
		return NewKubernetesClient(explicit, os.Getenv("KUBERNETES_BEARER_TOKEN"), nil)
	}
	if host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT"); host != "" && port != "" {
		token, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
		if err != nil {
			return nil, &UnavailableError{Err: err}
		}
		return NewKubernetesClient("https://"+host+":"+port, strings.TrimSpace(string(token)), &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}})
	}
	return clientFromKubeconfig()
}

func (c *KubernetesClient) SubmitPipelineRun(ctx context.Context, input PipelineRunInput) (string, error) {
	manifest, err := RenderManifest(input)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(manifest)
	path := fmt.Sprintf("/apis/%s/%s/namespaces/%s/%s", SparkGroup, SparkVersion, input.Namespace, SparkPlural)
	var created map[string]any
	if err := c.doJSON(ctx, http.MethodPost, path, body, &created); err != nil {
		return "", err
	}
	if meta, _ := created["metadata"].(map[string]any); meta != nil {
		if name, _ := meta["name"].(string); name != "" {
			return name, nil
		}
	}
	return SparkAppName(input.PipelineID, input.RunID)
}

func (c *KubernetesClient) GetPipelineRunStatus(ctx context.Context, namespace, name string) (*SparkRunStatusReport, error) {
	path := fmt.Sprintf("/apis/%s/%s/namespaces/%s/%s/%s", SparkGroup, SparkVersion, namespace, SparkPlural, name)
	var obj map[string]any
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &obj); err != nil {
		var kube *KubeError
		if errors.As(err, &kube) && kube.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}
	report := ParseStatus(obj)
	return &report, nil
}

func (c *KubernetesClient) doJSON(ctx context.Context, method, path string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.BearerToken)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return &KubeError{Message: err.Error()}
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &KubeError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return &RenderError{Message: "kubernetes response JSON parse: " + err.Error()}
		}
	}
	return nil
}

func ParseStatus(body map[string]any) SparkRunStatusReport {
	statusObj, _ := body["status"].(map[string]any)
	appState, _ := statusObj["applicationState"].(map[string]any)
	state, _ := appState["state"].(string)
	errMsg, _ := appState["errorMessage"].(string)
	var errPtr *string
	if errMsg != "" {
		errPtr = &errMsg
	}
	status := SparkRunUnknown
	switch state {
	case "", "SUBMITTED", "PENDING_SUBMISSION", "PENDING_RERUN":
		status = SparkRunSubmitted
	case "RUNNING", "INVALIDATING", "SUCCEEDING", "FAILING":
		status = SparkRunRunning
	case "COMPLETED":
		status = SparkRunSucceeded
	case "FAILED", "FAILED_SUBMISSION":
		status = SparkRunFailed
	case "UNKNOWN":
		status = SparkRunUnknown
	default:
		status = SparkRunUnknown
	}
	return SparkRunStatusReport{Status: status, ErrorMessage: errPtr}
}

func normalizeYAML(v any) any {
	switch x := v.(type) {
	case map[string]any:
		m := map[string]any{}
		for k, v := range x {
			m[k] = normalizeYAML(v)
		}
		return m
	case map[any]any:
		m := map[string]any{}
		for k, v := range x {
			m[fmt.Sprint(k)] = normalizeYAML(v)
		}
		return m
	case []any:
		for i, v := range x {
			x[i] = normalizeYAML(v)
		}
		return x
	default:
		return x
	}
}

func firstUnsubstitutedPlaceholder(s string) string {
	start := strings.Index(s, "${")
	for start >= 0 {
		rest := s[start+2:]
		end := strings.Index(rest, "}")
		if end < 0 {
			return ""
		}
		name := rest[:end]
		ok := name != ""
		for _, r := range name {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
				ok = false
				break
			}
		}
		if ok {
			return name
		}
		next := strings.Index(rest[end+1:], "${")
		if next < 0 {
			return ""
		}
		start += 2 + end + 1 + next
	}
	return ""
}

type kubeconfig struct {
	CurrentContext string `yaml:"current-context"`
	Clusters       []struct {
		Name    string `yaml:"name"`
		Cluster struct {
			Server                   string `yaml:"server"`
			CertificateAuthority     string `yaml:"certificate-authority"`
			CertificateAuthorityData string `yaml:"certificate-authority-data"`
			InsecureSkipTLSVerify    bool   `yaml:"insecure-skip-tls-verify"`
		} `yaml:"cluster"`
	} `yaml:"clusters"`
	Users []struct {
		Name string `yaml:"name"`
		User struct {
			Token string `yaml:"token"`
		} `yaml:"user"`
	} `yaml:"users"`
	Contexts []struct {
		Name    string `yaml:"name"`
		Context struct {
			Cluster string `yaml:"cluster"`
			User    string `yaml:"user"`
		} `yaml:"context"`
	} `yaml:"contexts"`
}

func clientFromKubeconfig() (*KubernetesClient, error) {
	path := os.Getenv("KUBECONFIG")
	if path == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			path = filepath.Join(home, ".kube", "config")
		}
	}
	if path == "" {
		return nil, &UnavailableError{}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, &UnavailableError{Err: err}
	}
	var cfg kubeconfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, &UnavailableError{Err: err}
	}
	ctxName := cfg.CurrentContext
	if ctxName == "" && len(cfg.Contexts) > 0 {
		ctxName = cfg.Contexts[0].Name
	}
	var clusterName, userName string
	for _, ctx := range cfg.Contexts {
		if ctx.Name == ctxName {
			clusterName, userName = ctx.Context.Cluster, ctx.Context.User
			break
		}
	}
	var server, token string
	var selectedCluster *struct {
		Server                   string `yaml:"server"`
		CertificateAuthority     string `yaml:"certificate-authority"`
		CertificateAuthorityData string `yaml:"certificate-authority-data"`
		InsecureSkipTLSVerify    bool   `yaml:"insecure-skip-tls-verify"`
	}
	for i := range cfg.Clusters {
		if cfg.Clusters[i].Name == clusterName {
			server = cfg.Clusters[i].Cluster.Server
			selectedCluster = &cfg.Clusters[i].Cluster
			break
		}
	}
	for _, u := range cfg.Users {
		if u.Name == userName {
			token = u.User.Token
			break
		}
	}
	if server == "" {
		return nil, &UnavailableError{}
	}
	httpClient, err := httpClientFromKubeCluster(selectedCluster, filepath.Dir(path))
	if err != nil {
		return nil, &UnavailableError{Err: err}
	}
	return NewKubernetesClient(server, token, httpClient)
}

func httpClientFromKubeCluster(cluster *struct {
	Server                   string `yaml:"server"`
	CertificateAuthority     string `yaml:"certificate-authority"`
	CertificateAuthorityData string `yaml:"certificate-authority-data"`
	InsecureSkipTLSVerify    bool   `yaml:"insecure-skip-tls-verify"`
}, kubeconfigDir string) (*http.Client, error) {
	if cluster == nil {
		return nil, nil
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if cluster.InsecureSkipTLSVerify {
		tlsCfg.InsecureSkipVerify = true //nolint:gosec // Mirrors explicit kubeconfig opt-out for local test clusters.
	}
	if strings.TrimSpace(cluster.CertificateAuthorityData) != "" {
		pemBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cluster.CertificateAuthorityData))
		if err != nil {
			return nil, fmt.Errorf("decode kubeconfig certificate-authority-data: %w", err)
		}
		pool, err := certPoolFromPEM(pemBytes)
		if err != nil {
			return nil, err
		}
		tlsCfg.RootCAs = pool
	} else if strings.TrimSpace(cluster.CertificateAuthority) != "" {
		caPath := cluster.CertificateAuthority
		if !filepath.IsAbs(caPath) {
			caPath = filepath.Join(kubeconfigDir, caPath)
		}
		pemBytes, err := os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("read kubeconfig certificate-authority: %w", err)
		}
		pool, err := certPoolFromPEM(pemBytes)
		if err != nil {
			return nil, err
		}
		tlsCfg.RootCAs = pool
	}
	if tlsCfg.RootCAs == nil && !tlsCfg.InsecureSkipVerify {
		return nil, nil
	}
	return &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{TLSClientConfig: tlsCfg}}, nil
}

func certPoolFromPEM(pemBytes []byte) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		return nil, errors.New("kubeconfig certificate authority did not contain a valid PEM certificate")
	}
	return pool, nil
}

// escapeYAMLString prepares a string value to be substituted inside a
// double-quoted YAML scalar in the SparkApplication template. Backslashes and
// double-quotes have to be escaped so SQL like `WHERE col = "x"` does not break
// the surrounding YAML, and newlines are flattened so the inline-sql argument
// stays on one line for the spark-submit CLI.
func escapeYAMLString(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	return value
}

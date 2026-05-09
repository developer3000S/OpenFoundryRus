package spark

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func sampleInput() PipelineRunInput {
	return PipelineRunInput{PipelineID: "p-7c1a", RunID: "r-stub", Namespace: "openfoundry-spark", ApplicationType: SparkApplicationScala, PipelineRunnerImage: "localhost:5001/pipeline-runner:0.1.0", InputDatasetRID: "ri.dataset.main.in", OutputDatasetRID: "ri.dataset.main.out"}
}

func TestRenderManifestSparkApplicationContract(t *testing.T) {
	manifest, err := RenderManifest(sampleInput())
	require.NoError(t, err)
	require.Equal(t, SparkGroup+"/"+SparkVersion, manifest["apiVersion"])
	require.Equal(t, SparkKind, manifest["kind"])

	meta := manifest["metadata"].(map[string]any)
	require.Equal(t, "pipeline-run-p-7c1a-r-stub", meta["name"])
	require.Equal(t, "openfoundry-spark", meta["namespace"])

	spec := manifest["spec"].(map[string]any)
	require.Equal(t, "Scala", spec["type"])
	require.Equal(t, "cluster", spec["mode"])
	require.Equal(t, "localhost:5001/pipeline-runner:0.1.0", spec["image"])
	require.Equal(t, "com.openfoundry.pipeline.PipelineRunner", spec["mainClass"])
	require.Equal(t, "local:///opt/spark/jars/pipeline-runner-spark.jar", spec["mainApplicationFile"])
	args, _ := spec["arguments"].([]any)
	require.Contains(t, args, "ri.dataset.main.in")
	require.Contains(t, args, "ri.dataset.main.out")
	require.Contains(t, args, "--input-dataset")
	require.Contains(t, args, "--output-dataset")

	driver := spec["driver"].(map[string]any)
	require.Equal(t, 1, driver["cores"])
	require.Equal(t, "1g", driver["memory"])
	require.Equal(t, "spark", driver["serviceAccount"])
	executor := spec["executor"].(map[string]any)
	require.Equal(t, 1, executor["cores"])
	require.Equal(t, 2, executor["instances"])
	require.Equal(t, "2g", executor["memory"])

	encoded, err := json.Marshal(manifest)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "${")
}

func TestRenderManifestPythonContractWithOverrides(t *testing.T) {
	input := sampleInput()
	input.ApplicationType = SparkApplicationPython
	input.Resources = SparkResourceOverrides{DriverCores: 2, DriverMemory: "3g", ExecutorCores: 4, ExecutorInstances: 5, ExecutorMemory: "6g"}
	manifest, err := RenderManifest(input)
	require.NoError(t, err)
	spec := manifest["spec"].(map[string]any)
	require.Equal(t, "Python", spec["type"])
	require.Equal(t, "", spec["mainClass"])
	require.Equal(t, "local:///opt/spark/work-dir/pipeline_runner.py", spec["mainApplicationFile"])
	require.Equal(t, 2, spec["driver"].(map[string]any)["cores"])
	require.Equal(t, "3g", spec["driver"].(map[string]any)["memory"])
	require.Equal(t, 4, spec["executor"].(map[string]any)["cores"])
	require.Equal(t, 5, spec["executor"].(map[string]any)["instances"])
	require.Equal(t, "6g", spec["executor"].(map[string]any)["memory"])
}

func TestKubernetesClientSubmitPostsSparkApplicationContract(t *testing.T) {
	var gotPath, gotMethod, gotAuth, gotAccept, gotContentType string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		gotContentType = r.Header.Get("Content-Type")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer server.Close()
	client, err := NewKubernetesClient(server.URL, "token", server.Client())
	require.NoError(t, err)

	name, err := client.SubmitPipelineRun(context.Background(), sampleInput())
	require.NoError(t, err)
	require.Equal(t, "pipeline-run-p-7c1a-r-stub", name)
	require.Equal(t, http.MethodPost, gotMethod)
	require.Equal(t, "/apis/sparkoperator.k8s.io/v1beta2/namespaces/openfoundry-spark/sparkapplications", gotPath)
	require.Equal(t, "Bearer token", gotAuth)
	require.Equal(t, "application/json", gotAccept)
	require.Equal(t, "application/json", gotContentType)
	require.Equal(t, "sparkoperator.k8s.io/v1beta2", gotBody["apiVersion"])
	require.Equal(t, "SparkApplication", gotBody["kind"])
	meta := gotBody["metadata"].(map[string]any)
	require.Equal(t, "pipeline-run-p-7c1a-r-stub", meta["name"])
	require.Equal(t, "openfoundry-spark", meta["namespace"])
	spec := gotBody["spec"].(map[string]any)
	require.Equal(t, "Scala", spec["type"])
	require.Contains(t, spec["arguments"].([]any), "ri.dataset.main.in")
	require.Contains(t, spec["arguments"].([]any), "ri.dataset.main.out")
}

func TestKubernetesClientGetRunStatusPathAuthAndNotFound(t *testing.T) {
	seen := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = r.Header.Get("Authorization")
		if strings.HasSuffix(r.URL.Path, "/missing") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": map[string]any{"applicationState": map[string]any{"state": "RUNNING"}}})
	}))
	defer server.Close()
	client, err := NewKubernetesClient(server.URL, "status-token", server.Client())
	require.NoError(t, err)

	report, err := client.GetPipelineRunStatus(context.Background(), "ns", "present")
	require.NoError(t, err)
	require.NotNil(t, report)
	require.Equal(t, SparkRunRunning, report.Status)
	presentPath := "/apis/sparkoperator.k8s.io/v1beta2/namespaces/ns/sparkapplications/present"
	require.Equal(t, "Bearer status-token", seen[presentPath])

	report, err = client.GetPipelineRunStatus(context.Background(), "ns", "missing")
	require.NoError(t, err)
	require.Nil(t, report)
	missingPath := "/apis/sparkoperator.k8s.io/v1beta2/namespaces/ns/sparkapplications/missing"
	require.Equal(t, "Bearer status-token", seen[missingPath])
}

func TestClientFromKubeconfigUsesTokenAndCertificateAuthorityData(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer kubeconfig-token", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]any{"status": map[string]any{"applicationState": map[string]any{"state": "COMPLETED"}}})
	}))
	defer server.Close()

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	require.NotNil(t, certPEM)
	kubeconfig := `apiVersion: v1
kind: Config
current-context: test
clusters:
- name: cluster
  cluster:
    server: ` + server.URL + `
    certificate-authority-data: ` + base64.StdEncoding.EncodeToString(certPEM) + `
users:
- name: user
  user:
    token: kubeconfig-token
contexts:
- name: test
  context:
    cluster: cluster
    user: user
`
	path := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(path, []byte(kubeconfig), 0o600))
	t.Setenv("KUBECONFIG", path)
	t.Setenv("KUBERNETES_API_URL", "")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")

	client, err := NewKubernetesClientFromEnv()
	require.NoError(t, err)
	require.Equal(t, server.URL, client.BaseURL)
	require.Equal(t, "kubeconfig-token", client.BearerToken)

	report, err := client.GetPipelineRunStatus(context.Background(), "ns", "name")
	require.NoError(t, err)
	require.Equal(t, SparkRunSucceeded, report.Status)
}

func TestClientFromKubeconfigRejectsInvalidCertificateAuthorityData(t *testing.T) {
	kubeconfig := `apiVersion: v1
current-context: test
clusters:
- name: cluster
  cluster:
    server: https://127.0.0.1:6443
    certificate-authority-data: not-base64
users:
- name: user
  user:
    token: token
contexts:
- name: test
  context:
    cluster: cluster
    user: user
`
	path := filepath.Join(t.TempDir(), "config")
	require.NoError(t, os.WriteFile(path, []byte(kubeconfig), 0o600))
	t.Setenv("KUBECONFIG", path)
	t.Setenv("KUBERNETES_API_URL", "")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")

	_, err := NewKubernetesClientFromEnv()
	require.Error(t, err)
	var unavailable *UnavailableError
	require.ErrorAs(t, err, &unavailable)
}

func TestKubeconfigCertPoolRejectsNonCertificatePEM(t *testing.T) {
	_, err := certPoolFromPEM([]byte("-----BEGIN NOT A CERT-----\n-----END NOT A CERT-----\n"))
	require.Error(t, err)
}

func TestCertPoolFromPEMAcceptsCertificate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer server.Close()
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	pool, err := certPoolFromPEM(certPEM)
	require.NoError(t, err)
	require.NotNil(t, pool)
	_, err = x509.ParseCertificate(server.Certificate().Raw)
	require.NoError(t, err)
}

func TestRenderManifestRejectsInvalidSpec(t *testing.T) {
	bad := sampleInput()
	bad.PipelineRunnerImage = ""
	_, err := RenderManifest(bad)
	require.Error(t, err)
	var invalid *InvalidInputError
	require.ErrorAs(t, err, &invalid)
	require.Contains(t, invalid.Message, "pipeline_runner_image")
}

# Rust vs Go HTTP route parity audit

Date: 2026-05-07

Generated with:

```sh
python3 tools/http_route_audit.py --write openfoundry-go/HTTP-ROUTE-AUDIT.md
```

State values:

- `implemented`: route exists in Rust and Go and the Go handler is not detected as a placeholder.
- `empty envelope`: Go route exists but returns a placeholder empty/list envelope.
- `501`: Go route exists but the handler advertises Not Implemented or equivalent pending behavior.
- `missing`: Rust route was not found in Go. A blank Rust handler (`—`) means the Go route was not found in the Rust route table (usually health/metrics aliases or newer Go-only surface).

This script is regex-based and optimized for the Axum/chi route declaration styles used in this repository; validate unusual dynamic route construction manually.

## pipeline-build-service

Rust routes: 24. Go routes: 29.
State counts: 501: 12, empty envelope: 6, implemented: 11, missing: 23.

| Route | Method | Rust handler | Go handler | State |
| --- | --- | --- | --- | --- |
| `/api/v1/builds` | GET | — | `handler.ListBuilds`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:73</sub> | empty envelope |
| `/api/v1/builds` | POST | — | `handler.CreateBuild`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:74</sub> | 501 |
| `/api/v1/builds/{id}` | GET | — | `handler.GetBuild`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:75</sub> | implemented |
| `/api/v1/builds/{id}/abort` | POST | — | `handler.AbortBuild`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:76</sub> | 501 |
| `/api/v1/builds/{id}/jobs` | GET | — | `handler.ListJobs`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:77</sub> | empty envelope |
| `/api/v1/data-integration/builds` | GET | `handlers::builds::list_builds`<br><sub>services/pipeline-build-service/src/main.rs:135</sub> | — | missing |
| `/api/v1/data-integration/builds/_summary` | GET | `handlers::builds::queue_summary`<br><sub>services/pipeline-build-service/src/main.rs:136</sub> | — | missing |
| `/api/v1/data-integration/builds/{run_id}/abort` | POST | `handlers::builds::abort_build`<br><sub>services/pipeline-build-service/src/main.rs:137</sub> | — | missing |
| `/api/v1/data-integration/pipelines/_scheduler/run-due` | POST | `handlers::execute::run_due_scheduled_pipelines`<br><sub>services/pipeline-build-service/src/main.rs:141</sub> | — | missing |
| `/api/v1/data-integration/pipelines/{pipeline_rid}/dry-run-resolve` | POST | `handlers::dry_run::dry_run_resolve`<br><sub>services/pipeline-build-service/src/main.rs:147</sub> | — | missing |
| `/api/v1/data-integration/pipelines/{id}/runs` | GET | `handlers::runs::list_runs`<br><sub>services/pipeline-build-service/src/main.rs:123</sub> | — | missing |
| `/api/v1/data-integration/pipelines/{id}/runs` | POST | `handlers::execute::trigger_run`<br><sub>services/pipeline-build-service/src/main.rs:123</sub> | — | missing |
| `/api/v1/data-integration/pipelines/{id}/runs/{run_id}` | GET | `handlers::runs::get_run`<br><sub>services/pipeline-build-service/src/main.rs:127</sub> | — | missing |
| `/api/v1/data-integration/pipelines/{id}/runs/{run_id}/retry` | POST | `handlers::execute::retry_run`<br><sub>services/pipeline-build-service/src/main.rs:131</sub> | — | missing |
| `/api/v1/data-integration/pipelines/{id}/runs/{run_id}/spec` | GET | — | `handler.GetSpecForRun`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:108</sub> | implemented |
| `/api/v1/data-integration/spark-runs` | GET | — | `handler.ListSparkRuns`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:105</sub> | empty envelope |
| `/api/v1/data-integration/spark-runs` | POST | — | `handler.SubmitSparkRun`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:106</sub> | 501 |
| `/api/v1/data-integration/spark-runs/{id}` | GET | — | `handler.GetSparkRun`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:107</sub> | implemented |
| `/api/v1/dry-run/resolve` | POST | — | `handler.DryRunResolve`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:85</sub> | 501 |
| `/api/v1/dry-run/validate` | POST | — | `handler.DryRunValidate`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:86</sub> | 501 |
| `/api/v1/execute` | POST | — | `handler.ExecutePipeline`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:87</sub> | 501 |
| `/api/v1/jobs/{id}` | GET | — | `handler.GetJob`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:80</sub> | implemented |
| `/api/v1/jobs/{id}/logs` | GET | — | `handler.ListJobLogs`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:81</sub> | empty envelope |
| `/api/v1/jobs/{id}/logs/stream` | GET | — | `handler.StreamJobLogs`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:82</sub> | implemented |
| `/api/v1/pipeline/builds/run` | POST | `handlers::spark_runs::submit_pipeline_run`<br><sub>services/pipeline-build-service/src/main.rs:205</sub> | — | missing |
| `/api/v1/pipeline/builds/{run_id}/status` | GET | `handlers::spark_runs::get_pipeline_run_status`<br><sub>services/pipeline-build-service/src/main.rs:209</sub> | — | missing |
| `/api/v1/pipelines` | GET | — | `handler.ListPipelines`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:90</sub> | empty envelope |
| `/api/v1/pipelines` | POST | — | `handler.CreatePipeline`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:91</sub> | 501 |
| `/api/v1/pipelines/{id}` | DELETE | — | `handler.DeletePipeline`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:95</sub> | implemented |
| `/api/v1/pipelines/{id}` | GET | — | `handler.GetPipeline`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:92</sub> | implemented |
| `/api/v1/pipelines/{id}` | PATCH | — | `handler.UpdatePipeline`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:93</sub> | 501 |
| `/api/v1/pipelines/{id}` | PUT | — | `handler.UpdatePipeline`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:94</sub> | 501 |
| `/api/v1/pipelines/{id}/runs` | GET | — | `handler.ListPipelineRuns`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:97</sub> | empty envelope |
| `/api/v1/pipelines/{id}/runs` | POST | — | `handler.TriggerPipelineRun`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:98</sub> | 501 |
| `/api/v1/pipelines/{id}/runs/{run_id}` | GET | — | `handler.GetPipelineRun`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:99</sub> | implemented |
| `/api/v1/pipelines/{id}/runs/{run_id}/cancel` | POST | — | `handler.CancelPipelineRun`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:101</sub> | 501 |
| `/api/v1/pipelines/{id}/runs/{run_id}/retry` | POST | — | `handler.RetryPipelineRun`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:100</sub> | 501 |
| `/health` | GET | — | `func(w`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:60</sub> | implemented |
| `/healthz` | GET | `||`<br><sub>services/pipeline-build-service/src/main.rs:218</sub> | `func(w`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:56</sub> | implemented |
| `/metrics` | GET | — | `m.Handler(`<br><sub>openfoundry-go/services/pipeline-build-service/internal/server/server.go:65</sub> | implemented |
| `/v1/builds` | GET | `handlers::builds_v1::list_builds_v1`<br><sub>services/pipeline-build-service/src/main.rs:157</sub> | — | missing |
| `/v1/builds` | POST | `handlers::builds_v1::create_build`<br><sub>services/pipeline-build-service/src/main.rs:157</sub> | — | missing |
| `/v1/builds/{rid}` | GET | `handlers::builds_v1::get_build`<br><sub>services/pipeline-build-service/src/main.rs:161</sub> | — | missing |
| `/v1/builds/{rid}:abort` | POST | `handlers::builds_v1::abort_build_v1`<br><sub>services/pipeline-build-service/src/main.rs:162</sub> | — | missing |
| `/v1/datasets/{rid}/builds` | GET | `handlers::builds_v1::list_dataset_builds`<br><sub>services/pipeline-build-service/src/main.rs:166</sub> | — | missing |
| `/v1/job-specs/{kind}` | POST | `handlers::builds_v1::create_job_spec`<br><sub>services/pipeline-build-service/src/main.rs:178</sub> | — | missing |
| `/v1/jobs/{rid}/input-resolutions` | GET | `handlers::builds_v1::get_job_input_resolutions`<br><sub>services/pipeline-build-service/src/main.rs:174</sub> | — | missing |
| `/v1/jobs/{rid}/logs` | GET | `handlers::job_logs::list_logs`<br><sub>services/pipeline-build-service/src/main.rs:183</sub> | — | missing |
| `/v1/jobs/{rid}/logs` | POST | `handlers::job_logs::emit_log`<br><sub>services/pipeline-build-service/src/main.rs:183</sub> | — | missing |
| `/v1/jobs/{rid}/logs/stream` | GET | `handlers::job_logs::stream_logs`<br><sub>services/pipeline-build-service/src/main.rs:187</sub> | — | missing |
| `/v1/jobs/{rid}/logs/ws` | GET | `handlers::job_logs::ws_logs`<br><sub>services/pipeline-build-service/src/main.rs:191</sub> | — | missing |
| `/v1/jobs/{rid}/outputs` | GET | `handlers::builds_v1::get_job_outputs`<br><sub>services/pipeline-build-service/src/main.rs:170</sub> | — | missing |

## notebook-runtime-service

Rust routes: 0. Go routes: 40.
State counts: 501: 5, empty envelope: 4, implemented: 19.

| Route | Method | Rust handler | Go handler | State |
| --- | --- | --- | --- | --- |
| `/api/v1/notebooks` | GET | — | `state.ListNotebooks`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:58</sub> | empty envelope |
| `/api/v1/notebooks` | POST | — | `state.CreateNotebook`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:59</sub> | implemented |
| `/api/v1/notebooks/{notebook_id}` | DELETE | — | `state.DeleteNotebook`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:63</sub> | implemented |
| `/api/v1/notebooks/{notebook_id}` | GET | — | `state.GetNotebook`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:60</sub> | implemented |
| `/api/v1/notebooks/{notebook_id}` | PATCH | — | `state.UpdateNotebook`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:62</sub> | implemented |
| `/api/v1/notebooks/{notebook_id}` | PUT | — | `state.UpdateNotebook`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:61</sub> | implemented |
| `/api/v1/notebooks/{notebook_id}/cells` | POST | — | `state.AddCell`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:66</sub> | implemented |
| `/api/v1/notebooks/{notebook_id}/cells/execute-all` | POST | — | `state.ExecuteAllCells`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:77</sub> | 501 |
| `/api/v1/notebooks/{notebook_id}/cells/{cell_id}` | DELETE | — | `state.DeleteCell`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:68</sub> | implemented |
| `/api/v1/notebooks/{notebook_id}/cells/{cell_id}` | PATCH | — | `state.UpdateCell`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:67</sub> | implemented |
| `/api/v1/notebooks/{notebook_id}/cells/{cell_id}/execute` | POST | — | `state.ExecuteCell`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:76</sub> | 501 |
| `/api/v1/notebooks/{notebook_id}/sessions` | GET | — | `state.ListSessions`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:71</sub> | empty envelope |
| `/api/v1/notebooks/{notebook_id}/sessions` | POST | — | `state.CreateSession`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:72</sub> | implemented |
| `/api/v1/notebooks/{notebook_id}/sessions/{session_id}/stop` | POST | — | `state.StopSession`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:73</sub> | implemented |
| `/api/v1/notebooks/{notebook_id}/workspace` | DELETE | — | `state.DeleteWorkspaceFile`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:82</sub> | implemented |
| `/api/v1/notebooks/{notebook_id}/workspace` | GET | — | `state.ListWorkspaceFiles`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:80</sub> | implemented |
| `/api/v1/notebooks/{notebook_id}/workspace` | PUT | — | `state.UpsertWorkspaceFile`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:81</sub> | implemented |
| `/api/v1/notepad/documents` | GET | — | `state.ListDocuments`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:85</sub> | empty envelope |
| `/api/v1/notepad/documents` | POST | — | `state.CreateDocument`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:86</sub> | 501 |
| `/api/v1/notepad/documents/{document_id}` | DELETE | — | `state.DeleteDocument`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:89</sub> | implemented |
| `/api/v1/notepad/documents/{document_id}` | GET | — | `state.GetDocument`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:87</sub> | implemented |
| `/api/v1/notepad/documents/{document_id}` | PATCH | — | `state.UpdateDocument`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:88</sub> | 501 |
| `/api/v1/notepad/documents/{document_id}/export` | POST | — | `state.ExportDocument`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:92</sub> | implemented |
| `/api/v1/notepad/documents/{document_id}/presence` | GET | — | `state.ListPresence`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:90</sub> | empty envelope |
| `/api/v1/notepad/documents/{document_id}/presence` | POST | — | `state.UpsertPresence`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:91</sub> | 501 |
| `/health` | GET | — | `func(w`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:45</sub> | implemented |
| `/healthz` | GET | — | `func(w`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:41</sub> | implemented |
| `/metrics` | GET | — | `m.Handler(`<br><sub>openfoundry-go/services/notebook-runtime-service/internal/server/server.go:50</sub> | implemented |

## ontology-query-service

Rust routes: 3. Go routes: 4.
State counts: 501: 2, implemented: 2, missing: 1.

| Route | Method | Rust handler | Go handler | State |
| --- | --- | --- | --- | --- |
| `/api/v1/ontology/objects/{tenant}/by-type/{type_id}` | GET | `handlers::list_objects_by_type`<br><sub>services/ontology-query-service/src/lib.rs:42</sub> | `h.ListObjectsByType`<br><sub>openfoundry-go/services/ontology-query-service/internal/server/server.go:38</sub> | 501 |
| `/api/v1/ontology/objects/{tenant}/{object_id}` | GET | `handlers::get_object`<br><sub>services/ontology-query-service/src/lib.rs:41</sub> | `h.GetObject`<br><sub>openfoundry-go/services/ontology-query-service/internal/server/server.go:37</sub> | 501 |
| `/health` | GET | `||`<br><sub>services/ontology-query-service/src/main.rs:79</sub> | — | missing |
| `/healthz` | GET | — | `func(w`<br><sub>openfoundry-go/services/ontology-query-service/internal/server/server.go:28</sub> | implemented |
| `/metrics` | GET | — | `m.Handler(`<br><sub>openfoundry-go/services/ontology-query-service/internal/server/server.go:32</sub> | implemented |

## connector-management-service

Rust routes: 47. Go routes: 7.
State counts: implemented: 7, missing: 46.

| Route | Method | Rust handler | Go handler | State |
| --- | --- | --- | --- | --- |
| `/api/v1/connections` | GET | — | `h.ListConnections`<br><sub>openfoundry-go/services/connector-management-service/internal/server/server.go:37</sub> | implemented |
| `/api/v1/connections` | POST | — | `h.CreateConnection`<br><sub>openfoundry-go/services/connector-management-service/internal/server/server.go:38</sub> | implemented |
| `/api/v1/connections/{id}` | DELETE | — | `h.DeleteConnection`<br><sub>openfoundry-go/services/connector-management-service/internal/server/server.go:41</sub> | implemented |
| `/api/v1/connections/{id}` | GET | — | `h.GetConnection`<br><sub>openfoundry-go/services/connector-management-service/internal/server/server.go:39</sub> | implemented |
| `/api/v1/connections/{id}` | PATCH | — | `h.UpdateConnection`<br><sub>openfoundry-go/services/connector-management-service/internal/server/server.go:40</sub> | implemented |
| `/auth/bootstrap-status` | GET | `handlers::dev_auth::bootstrap_status`<br><sub>services/connector-management-service/src/main.rs:281</sub> | — | missing |
| `/auth/login` | POST | `handlers::dev_auth::login`<br><sub>services/connector-management-service/src/main.rs:279</sub> | — | missing |
| `/auth/refresh` | POST | `handlers::dev_auth::refresh`<br><sub>services/connector-management-service/src/main.rs:280</sub> | — | missing |
| `/connections` | GET | `handlers::connections::list_connections`<br><sub>services/connector-management-service/src/main.rs:244</sub> | — | missing |
| `/connections` | POST | `handlers::connections::create_connection`<br><sub>services/connector-management-service/src/main.rs:244</sub> | — | missing |
| `/connections/{id}` | DELETE | `handlers::connections::delete_connection`<br><sub>services/connector-management-service/src/main.rs:249</sub> | — | missing |
| `/connections/{id}` | GET | `handlers::connections::get_connection`<br><sub>services/connector-management-service/src/main.rs:249</sub> | — | missing |
| `/connections/{id}/test` | POST | `handlers::connections::test_connection`<br><sub>services/connector-management-service/src/main.rs:254</sub> | — | missing |
| `/data-connection/catalog` | GET | `handlers::catalog::get_connector_catalog`<br><sub>services/connector-management-service/src/main.rs:113</sub> | — | missing |
| `/data-connection/catalog/contracts` | GET | `handlers::catalog::get_connector_contracts`<br><sub>services/connector-management-service/src/main.rs:114</sub> | — | missing |
| `/data-connection/sources` | GET | `handlers::connections::list_connections`<br><sub>services/connector-management-service/src/main.rs:123</sub> | — | missing |
| `/data-connection/sources` | POST | `handlers::connections::create_connection`<br><sub>services/connector-management-service/src/main.rs:123</sub> | — | missing |
| `/data-connection/sources/{id}` | DELETE | `handlers::connections::delete_connection`<br><sub>services/connector-management-service/src/main.rs:128</sub> | — | missing |
| `/data-connection/sources/{id}` | GET | `handlers::connections::get_connection`<br><sub>services/connector-management-service/src/main.rs:128</sub> | — | missing |
| `/data-connection/sources/{id}/capabilities` | GET | `handlers::catalog::get_connection_capabilities`<br><sub>services/connector-management-service/src/main.rs:137</sub> | — | missing |
| `/data-connection/sources/{id}/credentials` | GET | `handlers::data_connection::list_credentials`<br><sub>services/connector-management-service/src/main.rs:141</sub> | — | missing |
| `/data-connection/sources/{id}/credentials` | POST | `handlers::data_connection::set_credential`<br><sub>services/connector-management-service/src/main.rs:141</sub> | — | missing |
| `/data-connection/sources/{id}/egress-policies` | GET | `handlers::data_connection::list_source_policies`<br><sub>services/connector-management-service/src/main.rs:146</sub> | — | missing |
| `/data-connection/sources/{id}/egress-policies` | POST | `handlers::data_connection::attach_policy`<br><sub>services/connector-management-service/src/main.rs:146</sub> | — | missing |
| `/data-connection/sources/{source_id}/egress-policies/{policy_id}` | DELETE | `handlers::data_connection::detach_policy`<br><sub>services/connector-management-service/src/main.rs:151</sub> | — | missing |
| `/data-connection/sources/{id}/media-set-syncs` | GET | `handlers::media_set_syncs::list_media_set_syncs`<br><sub>services/connector-management-service/src/main.rs:166</sub> | — | missing |
| `/data-connection/sources/{id}/media-set-syncs` | POST | `handlers::media_set_syncs::create_media_set_sync`<br><sub>services/connector-management-service/src/main.rs:166</sub> | — | missing |
| `/data-connection/sources/{id}/registrations` | GET | `handlers::registrations::list_registrations`<br><sub>services/connector-management-service/src/main.rs:175</sub> | — | missing |
| `/data-connection/sources/{id}/registrations/auto` | POST | `handlers::registrations::auto_register`<br><sub>services/connector-management-service/src/main.rs:191</sub> | — | missing |
| `/data-connection/sources/{id}/registrations/auto` | PUT | `handlers::registrations::update_auto_registration`<br><sub>services/connector-management-service/src/main.rs:195</sub> | — | missing |
| `/data-connection/sources/{id}/registrations/auto/status` | GET | `handlers::registrations::auto_register_status`<br><sub>services/connector-management-service/src/main.rs:199</sub> | — | missing |
| `/data-connection/sources/{id}/registrations/bulk` | POST | `handlers::registrations::bulk_register`<br><sub>services/connector-management-service/src/main.rs:183</sub> | — | missing |
| `/data-connection/sources/{id}/registrations/bulk/preview` | POST | `handlers::registrations::bulk_register_preview`<br><sub>services/connector-management-service/src/main.rs:187</sub> | — | missing |
| `/data-connection/sources/{id}/registrations/discover` | POST | `handlers::registrations::discover`<br><sub>services/connector-management-service/src/main.rs:179</sub> | — | missing |
| `/data-connection/sources/{source_id}/registrations/{registration_id}` | DELETE | `handlers::registrations::delete_registration`<br><sub>services/connector-management-service/src/main.rs:203</sub> | — | missing |
| `/data-connection/sources/{source_id}/registrations/{registration_id}/query` | POST | `handlers::registrations::query_registration`<br><sub>services/connector-management-service/src/main.rs:207</sub> | — | missing |
| `/data-connection/sources/{source_id}/registrations/{registration_id}/query/arrow` | POST | `handlers::registrations::query_registration_arrow`<br><sub>services/connector-management-service/src/main.rs:211</sub> | — | missing |
| `/data-connection/sources/{id}/syncs` | GET | `handlers::data_connection::list_syncs`<br><sub>services/connector-management-service/src/main.rs:155</sub> | — | missing |
| `/data-connection/sources/{id}/test-connection` | POST | `handlers::connections::test_connection`<br><sub>services/connector-management-service/src/main.rs:133</sub> | — | missing |
| `/data-connection/streaming-sources` | GET | `handlers::streaming_syncs::list_streaming_sources`<br><sub>services/connector-management-service/src/main.rs:119</sub> | — | missing |
| `/data-connection/syncs` | POST | `handlers::data_connection::create_sync`<br><sub>services/connector-management-service/src/main.rs:159</sub> | — | missing |
| `/data-connection/syncs/{id}/run` | POST | `handlers::data_connection::run_sync`<br><sub>services/connector-management-service/src/main.rs:160</sub> | — | missing |
| `/data-connection/syncs/{id}/runs` | GET | `handlers::data_connection::list_runs`<br><sub>services/connector-management-service/src/main.rs:161</sub> | — | missing |
| `/health` | GET | `||`<br><sub>services/connector-management-service/src/main.rs:294</sub> | — | missing |
| `/healthz` | GET | — | `func(w`<br><sub>openfoundry-go/services/connector-management-service/internal/server/server.go:28</sub> | implemented |
| `/iceberg/v1/config` | GET | `handlers::iceberg_catalog::get_config`<br><sub>services/connector-management-service/src/main.rs:223</sub> | — | missing |
| `/iceberg/v1/namespaces` | GET | `handlers::iceberg_catalog::list_namespaces`<br><sub>services/connector-management-service/src/main.rs:224</sub> | — | missing |
| `/iceberg/v1/namespaces/{namespace}` | GET | `handlers::iceberg_catalog::get_namespace`<br><sub>services/connector-management-service/src/main.rs:228</sub> | — | missing |
| `/iceberg/v1/namespaces/{namespace}/tables` | GET | `handlers::iceberg_catalog::list_tables`<br><sub>services/connector-management-service/src/main.rs:232</sub> | — | missing |
| `/iceberg/v1/namespaces/{namespace}/tables/{table}` | GET | `handlers::iceberg_catalog::load_table`<br><sub>services/connector-management-service/src/main.rs:236</sub> | — | missing |
| `/metrics` | GET | `metrics_handler`<br><sub>services/connector-management-service/src/main.rs:295</sub> | `m.Handler(`<br><sub>openfoundry-go/services/connector-management-service/internal/server/server.go:32</sub> | implemented |
| `/users/me` | GET | `handlers::dev_auth::me`<br><sub>services/connector-management-service/src/main.rs:285</sub> | — | missing |
| `/webhooks/{id}/invoke` | POST | `handlers::webhooks::invoke_webhook`<br><sub>services/connector-management-service/src/main.rs:264</sub> | — | missing |

## dataset-versioning-service

Rust routes: 78. Go routes: 7.
State counts: implemented: 7, missing: 68.

| Route | Method | Rust handler | Go handler | State |
| --- | --- | --- | --- | --- |
| `/api/v1/datasets` | GET | — | `h.ListDatasets`<br><sub>openfoundry-go/services/dataset-versioning-service/internal/server/server.go:37</sub> | implemented |
| `/api/v1/datasets` | POST | — | `h.CreateDataset`<br><sub>openfoundry-go/services/dataset-versioning-service/internal/server/server.go:38</sub> | implemented |
| `/api/v1/datasets/{id}` | DELETE | — | `h.DeleteDataset`<br><sub>openfoundry-go/services/dataset-versioning-service/internal/server/server.go:41</sub> | implemented |
| `/api/v1/datasets/{id}` | GET | — | `h.GetDataset`<br><sub>openfoundry-go/services/dataset-versioning-service/internal/server/server.go:39</sub> | implemented |
| `/api/v1/datasets/{id}` | PATCH | — | `h.UpdateDataset`<br><sub>openfoundry-go/services/dataset-versioning-service/internal/server/server.go:40</sub> | implemented |
| `/api/v1/datasets/{rid}/health` | GET | `handlers::health::get_dataset_health`<br><sub>services/dataset-versioning-service/src/dataset_quality/mod.rs:66</sub> | — | missing |
| `/api/v1/datasets/{id}/lint` | GET | `handlers::lint::get_dataset_lint`<br><sub>services/dataset-versioning-service/src/dataset_quality/mod.rs:56</sub> | — | missing |
| `/api/v1/datasets/{id}/quality` | GET | `handlers::quality::get_dataset_quality`<br><sub>services/dataset-versioning-service/src/dataset_quality/mod.rs:38</sub> | — | missing |
| `/api/v1/datasets/{id}/quality/profile` | POST | `handlers::quality::refresh_dataset_quality`<br><sub>services/dataset-versioning-service/src/dataset_quality/mod.rs:42</sub> | — | missing |
| `/api/v1/datasets/{id}/quality/rules` | POST | `handlers::quality::create_quality_rule`<br><sub>services/dataset-versioning-service/src/dataset_quality/mod.rs:46</sub> | — | missing |
| `/api/v1/datasets/{id}/quality/rules/{rule_id}` | DELETE | `handlers::quality::delete_quality_rule`<br><sub>services/dataset-versioning-service/src/dataset_quality/mod.rs:50</sub> | — | missing |
| `/api/v1/datasets/{id}/quality/rules/{rule_id}` | PATCH | `handlers::quality::update_quality_rule`<br><sub>services/dataset-versioning-service/src/dataset_quality/mod.rs:50</sub> | — | missing |
| `/health` | GET | `handlers::health::healthz`<br><sub>services/dataset-versioning-service/src/lib.rs:226</sub> | — | missing |
| `/healthz` | GET | `handlers::health::healthz`<br><sub>services/dataset-versioning-service/src/lib.rs:225</sub> | `func(w`<br><sub>openfoundry-go/services/dataset-versioning-service/internal/server/server.go:28</sub> | implemented |
| `/internal/datasets/{rid}/metadata` | GET | `handlers::internal::get_dataset_metadata`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:137</sub> | — | missing |
| `/metrics` | GET | `handlers::health::metrics`<br><sub>services/dataset-versioning-service/src/lib.rs:227</sub> | `m.Handler(`<br><sub>openfoundry-go/services/dataset-versioning-service/internal/server/server.go:32</sub> | implemented |
| `/v1/_internal/local-fs/{*key}` | GET | `handlers::files::local_presign_proxy`<br><sub>services/dataset-versioning-service/src/lib.rs:228</sub> | — | missing |
| `/v1/catalog/facets` | GET | `handlers::catalog::get_catalog_facets`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:55</sub> | — | missing |
| `/v1/datasets` | GET | `handlers::crud::list_datasets`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:60</sub> | — | missing |
| `/v1/datasets` | POST | `handlers::crud::create_dataset`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:60</sub> | — | missing |
| `/v1/datasets/{rid}` | DELETE | `handlers::crud::delete_dataset`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:64</sub> | — | missing |
| `/v1/datasets/{rid}` | GET | `handlers::crud::get_dataset`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:64</sub> | — | missing |
| `/v1/datasets/{rid}` | PATCH | `handlers::crud::update_dataset`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:64</sub> | — | missing |
| `/v1/datasets/{rid}/branches` | GET | `handlers::foundry::list_branches`<br><sub>services/dataset-versioning-service/src/lib.rs:73</sub> | — | missing |
| `/v1/datasets/{rid}/branches` | POST | `handlers::foundry::create_branch`<br><sub>services/dataset-versioning-service/src/lib.rs:73</sub> | — | missing |
| `/v1/datasets/{rid}/branches/compare` | GET | `handlers::compare::compare_branches`<br><sub>services/dataset-versioning-service/src/lib.rs:120</sub> | — | missing |
| `/v1/datasets/{rid}/branches/{branch}` | DELETE | `handlers::foundry::delete_branch`<br><sub>services/dataset-versioning-service/src/lib.rs:84</sub> | — | missing |
| `/v1/datasets/{rid}/branches/{branch}` | GET | `handlers::foundry::get_branch`<br><sub>services/dataset-versioning-service/src/lib.rs:84</sub> | — | missing |
| `/v1/datasets/{rid}/branches/{branch}` | POST | `handlers::foundry::branch_action`<br><sub>services/dataset-versioning-service/src/lib.rs:84</sub> | — | missing |
| `/v1/datasets/{rid}/branches/{branch}/ancestry` | GET | `handlers::foundry::branch_ancestry`<br><sub>services/dataset-versioning-service/src/lib.rs:95</sub> | — | missing |
| `/v1/datasets/{rid}/branches/{branch}/checkout` | POST | `handlers::branches::checkout_branch`<br><sub>services/dataset-versioning-service/src/lib.rs:90</sub> | — | missing |
| `/v1/datasets/{rid}/branches/{branch}/fallbacks` | GET | `handlers::foundry::list_fallbacks`<br><sub>services/dataset-versioning-service/src/lib.rs:141</sub> | — | missing |
| `/v1/datasets/{rid}/branches/{branch}/fallbacks` | PUT | `handlers::foundry::put_fallbacks`<br><sub>services/dataset-versioning-service/src/lib.rs:141</sub> | — | missing |
| `/v1/datasets/{rid}/branches/{branch}/markings` | GET | `handlers::retention::get_branch_markings`<br><sub>services/dataset-versioning-service/src/lib.rs:110</sub> | — | missing |
| `/v1/datasets/{rid}/branches/{branch}/preview-delete` | GET | `handlers::foundry::preview_delete_branch`<br><sub>services/dataset-versioning-service/src/lib.rs:101</sub> | — | missing |
| `/v1/datasets/{rid}/branches/{branch}/retention` | PATCH | `handlers::retention::update_retention`<br><sub>services/dataset-versioning-service/src/lib.rs:106</sub> | — | missing |
| `/v1/datasets/{rid}/branches/{branch}/rollback` | POST | `handlers::foundry::rollback_branch`<br><sub>services/dataset-versioning-service/src/lib.rs:124</sub> | — | missing |
| `/v1/datasets/{rid}/branches/{branch}/transactions` | POST | `handlers::foundry::start_transaction`<br><sub>services/dataset-versioning-service/src/lib.rs:129</sub> | — | missing |
| `/v1/datasets/{rid}/branches/{branch}/transactions/{txn}` | GET | `handlers::foundry::get_transaction`<br><sub>services/dataset-versioning-service/src/lib.rs:135</sub> | — | missing |
| `/v1/datasets/{rid}/branches/{branch}/transactions/{txn}` | POST | `handlers::foundry::transaction_action`<br><sub>services/dataset-versioning-service/src/lib.rs:135</sub> | — | missing |
| `/v1/datasets/{rid}/branches/{branch}:restore` | POST | `handlers::retention::restore_branch`<br><sub>services/dataset-versioning-service/src/lib.rs:114</sub> | — | missing |
| `/v1/datasets/{rid}/compare` | GET | `handlers::foundry::compare_views`<br><sub>services/dataset-versioning-service/src/lib.rs:158</sub> | — | missing |
| `/v1/datasets/{rid}/files` | GET | `handlers::files::list_files`<br><sub>services/dataset-versioning-service/src/lib.rs:193</sub> | — | missing |
| `/v1/datasets/{rid}/files/index` | GET | `handlers::dataset_model::list_dataset_file_index`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:93</sub> | — | missing |
| `/v1/datasets/{rid}/files/index` | PUT | `handlers::dataset_model::put_dataset_file_index`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:93</sub> | — | missing |
| `/v1/datasets/{rid}/files/{file_id}/download` | GET | `handlers::files::download_file`<br><sub>services/dataset-versioning-service/src/lib.rs:197</sub> | — | missing |
| `/v1/datasets/{rid}/health` | GET | `handlers::health::get_dataset_health`<br><sub>services/dataset-versioning-service/src/dataset_quality/mod.rs:62</sub> | — | missing |
| `/v1/datasets/{rid}/lineage-links` | GET | `handlers::dataset_model::list_dataset_lineage_links`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:88</sub> | — | missing |
| `/v1/datasets/{rid}/lineage-links` | PUT | `handlers::dataset_model::put_dataset_lineage_links`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:88</sub> | — | missing |
| `/v1/datasets/{rid}/markings` | GET | `handlers::dataset_model::list_dataset_markings`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:78</sub> | — | missing |
| `/v1/datasets/{rid}/markings` | PUT | `handlers::dataset_model::put_dataset_markings`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:78</sub> | — | missing |
| `/v1/datasets/{rid}/metadata` | PATCH | `handlers::dataset_model::patch_dataset_metadata`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:74</sub> | — | missing |
| `/v1/datasets/{rid}/model` | GET | `handlers::dataset_model::get_dataset_model`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:70</sub> | — | missing |
| `/v1/datasets/{rid}/permissions` | GET | `handlers::dataset_model::list_dataset_permissions`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:83</sub> | — | missing |
| `/v1/datasets/{rid}/permissions` | PUT | `handlers::dataset_model::put_dataset_permissions`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:83</sub> | — | missing |
| `/v1/datasets/{rid}/preview` | GET | `handlers::preview::preview_data`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:99</sub> | — | missing |
| `/v1/datasets/{rid}/schema` | GET | `handlers::schema::get_current_schema`<br><sub>services/dataset-versioning-service/src/lib.rs:209</sub> | — | missing |
| `/v1/datasets/{rid}/schema:validate` | POST | `handlers::schema_validate::validate_schema`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:108</sub> | — | missing |
| `/v1/datasets/{rid}/storage-details` | GET | `handlers::files::storage_details`<br><sub>services/dataset-versioning-service/src/lib.rs:205</sub> | — | missing |
| `/v1/datasets/{rid}/transactions` | GET | `handlers::foundry::list_transactions`<br><sub>services/dataset-versioning-service/src/lib.rs:146</sub> | — | missing |
| `/v1/datasets/{rid}/transactions/{txn_id}/files` | POST | `handlers::files::upload_url`<br><sub>services/dataset-versioning-service/src/lib.rs:201</sub> | — | missing |
| `/v1/datasets/{rid}/transactions:batchGet` | POST | `handlers::foundry::batch_get_transactions`<br><sub>services/dataset-versioning-service/src/lib.rs:154</sub> | — | missing |
| `/v1/datasets/{rid}/upload` | POST | `handlers::upload::upload_data`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:113</sub> | — | missing |
| `/v1/datasets/{rid}/versions` | GET | `handlers::versions::list_versions`<br><sub>services/dataset-versioning-service/src/lib.rs:77</sub> | — | missing |
| `/v1/datasets/{rid}/views` | GET | `handlers::views::list_views`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:123</sub> | — | missing |
| `/v1/datasets/{rid}/views` | POST | `handlers::views::create_view`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:123</sub> | — | missing |
| `/v1/datasets/{rid}/views/at` | GET | `handlers::foundry::get_view_at`<br><sub>services/dataset-versioning-service/src/lib.rs:167</sub> | — | missing |
| `/v1/datasets/{rid}/views/current` | GET | `handlers::foundry::get_current_view`<br><sub>services/dataset-versioning-service/src/lib.rs:163</sub> | — | missing |
| `/v1/datasets/{rid}/views/{view_or_action}` | GET | `handlers::views::get_view`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:127</sub> | — | missing |
| `/v1/datasets/{rid}/views/{view_or_action}` | POST | `view_action_dispatch`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:127</sub> | — | missing |
| `/v1/datasets/{rid}/views/{view_id}/data` | GET | `handlers::preview::preview_view`<br><sub>services/dataset-versioning-service/src/lib.rs:187</sub> | — | missing |
| `/v1/datasets/{rid}/views/{view_id}/files` | GET | `handlers::foundry::list_view_files`<br><sub>services/dataset-versioning-service/src/lib.rs:171</sub> | — | missing |
| `/v1/datasets/{rid}/views/{view_id}/preview` | GET | `handlers::views::preview_view`<br><sub>services/dataset-versioning-service/src/data_asset_catalog/mod.rs:131</sub> | — | missing |
| `/v1/datasets/{rid}/views/{view_id}/schema` | GET | `handlers::schema::get_view_schema`<br><sub>services/dataset-versioning-service/src/lib.rs:179</sub> | — | missing |
| `/v1/datasets/{rid}/views/{view_id}/schema` | POST | `handlers::schema::put_view_schema`<br><sub>services/dataset-versioning-service/src/lib.rs:179</sub> | — | missing |

## ingestion-replication-service

Rust routes: 12. Go routes: 7.
State counts: empty envelope: 1, implemented: 6, missing: 12.

| Route | Method | Rust handler | Go handler | State |
| --- | --- | --- | --- | --- |
| `/api/v1/ingest-jobs` | GET | — | `h.ListIngestJobs`<br><sub>openfoundry-go/services/ingestion-replication-service/internal/server/server.go:37</sub> | empty envelope |
| `/api/v1/ingest-jobs` | POST | — | `h.CreateIngestJob`<br><sub>openfoundry-go/services/ingestion-replication-service/internal/server/server.go:38</sub> | implemented |
| `/api/v1/ingest-jobs/{id}` | DELETE | — | `h.DeleteIngestJob`<br><sub>openfoundry-go/services/ingestion-replication-service/internal/server/server.go:41</sub> | implemented |
| `/api/v1/ingest-jobs/{id}` | GET | — | `h.GetIngestJob`<br><sub>openfoundry-go/services/ingestion-replication-service/internal/server/server.go:39</sub> | implemented |
| `/api/v1/ingest-jobs/{id}` | PATCH | — | `h.UpdateIngestJob`<br><sub>openfoundry-go/services/ingestion-replication-service/internal/server/server.go:40</sub> | implemented |
| `/compatibility/subjects/:name/versions/:version` | POST | `check_compatibility`<br><sub>services/ingestion-replication-service/src/cdc_metadata/schema_registry.rs:47</sub> | — | missing |
| `/healthz` | GET | — | `func(w`<br><sub>openfoundry-go/services/ingestion-replication-service/internal/server/server.go:28</sub> | implemented |
| `/metrics` | GET | — | `m.Handler(`<br><sub>openfoundry-go/services/ingestion-replication-service/internal/server/server.go:32</sub> | implemented |
| `/streams` | GET | `handlers::list_streams`<br><sub>services/ingestion-replication-service/src/cdc_metadata/mod.rs:15</sub> | — | missing |
| `/streams` | POST | `handlers::register_stream`<br><sub>services/ingestion-replication-service/src/cdc_metadata/mod.rs:15</sub> | — | missing |
| `/streams/:id` | GET | `handlers::get_stream`<br><sub>services/ingestion-replication-service/src/cdc_metadata/mod.rs:19</sub> | — | missing |
| `/streams/:id/checkpoint` | GET | `handlers::get_checkpoint`<br><sub>services/ingestion-replication-service/src/cdc_metadata/mod.rs:20</sub> | — | missing |
| `/streams/:id/checkpoint` | POST | `handlers::record_checkpoint`<br><sub>services/ingestion-replication-service/src/cdc_metadata/mod.rs:20</sub> | — | missing |
| `/streams/:id/resolution` | GET | `handlers::get_resolution`<br><sub>services/ingestion-replication-service/src/cdc_metadata/mod.rs:24</sub> | — | missing |
| `/streams/:id/resolution` | PUT | `handlers::update_resolution`<br><sub>services/ingestion-replication-service/src/cdc_metadata/mod.rs:24</sub> | — | missing |
| `/subjects` | GET | `list_subjects`<br><sub>services/ingestion-replication-service/src/cdc_metadata/schema_registry.rs:41</sub> | — | missing |
| `/subjects/:name/versions` | GET | `list_versions`<br><sub>services/ingestion-replication-service/src/cdc_metadata/schema_registry.rs:42</sub> | — | missing |
| `/subjects/:name/versions` | POST | `register_version`<br><sub>services/ingestion-replication-service/src/cdc_metadata/schema_registry.rs:42</sub> | — | missing |
| `/subjects/:name/versions/:version` | GET | `get_version`<br><sub>services/ingestion-replication-service/src/cdc_metadata/schema_registry.rs:46</sub> | — | missing |

## iceberg-catalog-service

Rust routes: 29. Go routes: 7.
State counts: implemented: 7, missing: 28.

| Route | Method | Rust handler | Go handler | State |
| --- | --- | --- | --- | --- |
| `/api/v1/iceberg-tables` | GET | `handlers::admin::list_iceberg_tables`<br><sub>services/iceberg-catalog-service/src/lib.rs:119</sub> | — | missing |
| `/api/v1/iceberg-tables/{id}` | GET | `handlers::admin::get_iceberg_table_detail`<br><sub>services/iceberg-catalog-service/src/lib.rs:123</sub> | — | missing |
| `/api/v1/iceberg-tables/{id}/branches` | GET | `handlers::admin::list_iceberg_table_branches`<br><sub>services/iceberg-catalog-service/src/lib.rs:135</sub> | — | missing |
| `/api/v1/iceberg-tables/{id}/metadata` | GET | `handlers::admin::get_iceberg_table_metadata`<br><sub>services/iceberg-catalog-service/src/lib.rs:131</sub> | — | missing |
| `/api/v1/iceberg-tables/{id}/snapshots` | GET | `handlers::admin::list_iceberg_table_snapshots`<br><sub>services/iceberg-catalog-service/src/lib.rs:127</sub> | — | missing |
| `/api/v1/namespaces` | GET | — | `h.ListNamespaces`<br><sub>openfoundry-go/services/iceberg-catalog-service/internal/server/server.go:37</sub> | implemented |
| `/api/v1/namespaces` | POST | — | `h.CreateNamespace`<br><sub>openfoundry-go/services/iceberg-catalog-service/internal/server/server.go:38</sub> | implemented |
| `/api/v1/namespaces/{id}` | DELETE | — | `h.DeleteNamespace`<br><sub>openfoundry-go/services/iceberg-catalog-service/internal/server/server.go:41</sub> | implemented |
| `/api/v1/namespaces/{id}` | GET | — | `h.GetNamespace`<br><sub>openfoundry-go/services/iceberg-catalog-service/internal/server/server.go:39</sub> | implemented |
| `/api/v1/namespaces/{id}` | PATCH | — | `h.UpdateNamespace`<br><sub>openfoundry-go/services/iceberg-catalog-service/internal/server/server.go:40</sub> | implemented |
| `/health` | GET | `||`<br><sub>services/iceberg-catalog-service/src/lib.rs:144</sub> | — | missing |
| `/healthz` | GET | — | `func(w`<br><sub>openfoundry-go/services/iceberg-catalog-service/internal/server/server.go:28</sub> | implemented |
| `/iceberg/v1/config` | GET | `handlers::rest_catalog::config::get_config`<br><sub>services/iceberg-catalog-service/src/lib.rs:41</sub> | — | missing |
| `/iceberg/v1/diagnose` | POST | `handlers::diagnose::run_diagnose`<br><sub>services/iceberg-catalog-service/src/lib.rs:99</sub> | — | missing |
| `/iceberg/v1/namespaces` | GET | `handlers::rest_catalog::namespaces::list_namespaces`<br><sub>services/iceberg-catalog-service/src/lib.rs:46</sub> | — | missing |
| `/iceberg/v1/namespaces` | POST | `handlers::rest_catalog::namespaces::create_namespace`<br><sub>services/iceberg-catalog-service/src/lib.rs:46</sub> | — | missing |
| `/iceberg/v1/namespaces/{namespace}` | DELETE | `handlers::rest_catalog::namespaces::drop_namespace`<br><sub>services/iceberg-catalog-service/src/lib.rs:51</sub> | — | missing |
| `/iceberg/v1/namespaces/{namespace}` | GET | `handlers::rest_catalog::namespaces::load_namespace`<br><sub>services/iceberg-catalog-service/src/lib.rs:51</sub> | — | missing |
| `/iceberg/v1/namespaces/{namespace}/markings` | GET | `handlers::markings::get_namespace_markings`<br><sub>services/iceberg-catalog-service/src/lib.rs:88</sub> | — | missing |
| `/iceberg/v1/namespaces/{namespace}/markings` | POST | `handlers::markings::update_namespace_markings`<br><sub>services/iceberg-catalog-service/src/lib.rs:88</sub> | — | missing |
| `/iceberg/v1/namespaces/{namespace}/properties` | GET | `handlers::rest_catalog::namespaces::get_properties`<br><sub>services/iceberg-catalog-service/src/lib.rs:56</sub> | — | missing |
| `/iceberg/v1/namespaces/{namespace}/properties` | POST | `handlers::rest_catalog::namespaces::update_properties`<br><sub>services/iceberg-catalog-service/src/lib.rs:56</sub> | — | missing |
| `/iceberg/v1/namespaces/{namespace}/tables` | GET | `handlers::rest_catalog::tables::list_tables`<br><sub>services/iceberg-catalog-service/src/lib.rs:62</sub> | — | missing |
| `/iceberg/v1/namespaces/{namespace}/tables` | POST | `handlers::rest_catalog::tables::create_table`<br><sub>services/iceberg-catalog-service/src/lib.rs:62</sub> | — | missing |
| `/iceberg/v1/namespaces/{namespace}/tables/{table}` | DELETE | `handlers::rest_catalog::tables::drop_table`<br><sub>services/iceberg-catalog-service/src/lib.rs:67</sub> | — | missing |
| `/iceberg/v1/namespaces/{namespace}/tables/{table}` | GET | `handlers::rest_catalog::tables::load_table`<br><sub>services/iceberg-catalog-service/src/lib.rs:67</sub> | — | missing |
| `/iceberg/v1/namespaces/{namespace}/tables/{table}` | HEAD | `handlers::rest_catalog::tables::table_exists`<br><sub>services/iceberg-catalog-service/src/lib.rs:73</sub> | — | missing |
| `/iceberg/v1/namespaces/{namespace}/tables/{table}` | POST | `handlers::rest_catalog::tables::commit_table`<br><sub>services/iceberg-catalog-service/src/lib.rs:67</sub> | — | missing |
| `/iceberg/v1/namespaces/{namespace}/tables/{table}/alter-schema` | POST | `handlers::rest_catalog::tables::alter_schema`<br><sub>services/iceberg-catalog-service/src/lib.rs:78</sub> | — | missing |
| `/iceberg/v1/namespaces/{namespace}/tables/{table}/markings` | GET | `handlers::markings::get_table_markings`<br><sub>services/iceberg-catalog-service/src/lib.rs:93</sub> | — | missing |
| `/iceberg/v1/namespaces/{namespace}/tables/{table}/markings` | PATCH | `handlers::markings::update_table_markings`<br><sub>services/iceberg-catalog-service/src/lib.rs:93</sub> | — | missing |
| `/iceberg/v1/oauth/tokens` | POST | `handlers::auth::oauth::issue_token`<br><sub>services/iceberg-catalog-service/src/lib.rs:108</sub> | — | missing |
| `/iceberg/v1/transactions/commit` | POST | `handlers::rest_catalog::transactions::multi_table_commit`<br><sub>services/iceberg-catalog-service/src/lib.rs:83</sub> | — | missing |
| `/metrics` | GET | `metrics::render_metrics`<br><sub>services/iceberg-catalog-service/src/lib.rs:145</sub> | `m.Handler(`<br><sub>openfoundry-go/services/iceberg-catalog-service/internal/server/server.go:32</sub> | implemented |
| `/v1/iceberg-clients/api-tokens` | POST | `handlers::auth::api_tokens::create_api_token`<br><sub>services/iceberg-catalog-service/src/lib.rs:112</sub> | — | missing |

## federation-product-exchange-service

Rust routes: 23. Go routes: 2.
State counts: implemented: 2, missing: 21.

| Route | Method | Rust handler | Go handler | State |
| --- | --- | --- | --- | --- |
| `/health` | GET | `handlers::health::healthz`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:117</sub> | — | missing |
| `/healthz` | GET | `handlers::health::healthz`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:116</sub> | `func(w`<br><sub>openfoundry-go/services/federation-product-exchange-service/internal/server/server.go:44</sub> | implemented |
| `/metrics` | GET | `handlers::health::metrics`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:118</sub> | `m.Handler(`<br><sub>openfoundry-go/services/federation-product-exchange-service/internal/server/server.go:49</sub> | implemented |
| `/v1/marketplace/categories` | GET | `handlers::browse::list_categories`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:43</sub> | — | missing |
| `/v1/marketplace/installs` | GET | `handlers::install::list_installs`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:71</sub> | — | missing |
| `/v1/marketplace/installs` | POST | `handlers::install::create_install`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:71</sub> | — | missing |
| `/v1/marketplace/listings` | GET | `handlers::browse::list_listings`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:47</sub> | — | missing |
| `/v1/marketplace/listings` | POST | `handlers::publish::publish_listing`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:54</sub> | — | missing |
| `/v1/marketplace/listings/{id}` | GET | `handlers::browse::get_listing`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:48</sub> | — | missing |
| `/v1/marketplace/listings/{id}` | PATCH | `handlers::publish::update_listing`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:58</sub> | — | missing |
| `/v1/marketplace/listings/{id}/actions` | POST | `handlers::publish::include_action_in_product`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:66</sub> | — | missing |
| `/v1/marketplace/listings/{id}/versions` | GET | `handlers::publish::list_versions`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:62</sub> | — | missing |
| `/v1/marketplace/listings/{id}/versions` | POST | `handlers::publish::publish_version`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:62</sub> | — | missing |
| `/v1/marketplace/overview` | GET | `handlers::browse::get_overview`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:42</sub> | — | missing |
| `/v1/marketplace/products/from-dataset/{rid}` | POST | `handlers::dataset_product::create_from_dataset`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:93</sub> | — | missing |
| `/v1/marketplace/products/{id}` | GET | `handlers::dataset_product::get_dataset_product`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:97</sub> | — | missing |
| `/v1/marketplace/products/{id}/install` | POST | `handlers::dataset_product::install_dataset_product`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:101</sub> | — | missing |
| `/v1/marketplace/search` | GET | `handlers::browse::search_listings`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:52</sub> | — | missing |
| `/v1/products/from-dataset/{rid}` | POST | `handlers::dataset_product::create_from_dataset`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:81</sub> | — | missing |
| `/v1/products/{id}` | GET | `handlers::dataset_product::get_dataset_product`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:85</sub> | — | missing |
| `/v1/products/{id}/install` | POST | `handlers::dataset_product::install_dataset_product`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:89</sub> | — | missing |
| `/v1/products/{id}/install:schedules` | POST | `handlers::schedule_manifest::materialise_install_schedules`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:110</sub> | — | missing |
| `/v1/products/{id}/schedules` | POST | `handlers::schedule_manifest::add_schedule_manifest`<br><sub>services/federation-product-exchange-service/src/marketplace/mod.rs:106</sub> | — | missing |

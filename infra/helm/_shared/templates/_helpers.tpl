{{/*
Common labels for every OpenFoundry resource. Consumed by the five
release-aligned charts (of-platform, of-data-engine, of-ontology,
of-ml-aip, of-apps-ops).
*/}}
{{- define "of-shared.labels" -}}
{{- $root := .root | default dict -}}
{{- $global := $root.Values.global | default dict -}}
{{- $deploymentFabric := $global.deploymentFabric | default dict -}}
{{- $reservedLabels := dict "app.kubernetes.io/name" true "app.kubernetes.io/instance" true "app.kubernetes.io/managed-by" true "app.kubernetes.io/part-of" true "openfoundry.io/release" true "openfoundry.io/cloud" true "openfoundry.io/region" true "openfoundry.io/cell" true "openfoundry.io/environment" true "openfoundry.io/residency" true "openfoundry.io/apollo" true -}}
app.kubernetes.io/name: {{ .name }}
app.kubernetes.io/instance: {{ .release }}
app.kubernetes.io/managed-by: helm
app.kubernetes.io/part-of: openfoundry
openfoundry.io/release: {{ .release }}
{{- with $deploymentFabric.cloud }}
openfoundry.io/cloud: {{ . | quote }}
{{- end }}
{{- with $deploymentFabric.region }}
openfoundry.io/region: {{ . | quote }}
{{- end }}
{{- with $deploymentFabric.cell }}
openfoundry.io/cell: {{ . | quote }}
{{- end }}
{{- with $deploymentFabric.environment }}
openfoundry.io/environment: {{ . | quote }}
{{- end }}
{{- $geo := $deploymentFabric.geoRestrictions | default dict -}}
{{- if $geo.enabled }}
openfoundry.io/residency: {{ default "geo-restricted" $geo.residencyLabel | quote }}
{{- end }}
{{- if and $root.Values.apollo $root.Values.apollo.enabled }}
openfoundry.io/apollo: "enabled"
{{- end }}
{{- range $key, $value := ($global.labels | default dict) }}
{{- if not (hasKey $reservedLabels $key) }}
{{ $key }}: {{ $value | quote }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Stable name for the cross-release platform profile ConfigMap created by
of-platform and mounted by the other releases.
*/}}
{{- define "of-shared.platformProfileConfigMapName" -}}
{{- $root := .root | default . -}}
{{- default "openfoundry-platform-profile" (($root.Values.global.platformProfile | default dict).configMapName) -}}
{{- end -}}

{{/*
Default Apollo gateway URL. Can be overridden explicitly via
`.Values.apollo.gatewayUrl`.
*/}}
{{- define "of-shared.apolloGatewayUrl" -}}
{{- $root := .root | default . -}}
{{- if $root.Values.apollo.gatewayUrl -}}
{{- $root.Values.apollo.gatewayUrl -}}
{{- else -}}
{{- printf "http://%s:%v" (($root.Values.ingress | default dict).serviceName | default "edge-gateway-service") (($root.Values.ingress | default dict).servicePort | default 8080) -}}
{{- end -}}
{{- end -}}

{{/*
Build a full container image reference from chart-level defaults and an
optional service override.
*/}}
{{- define "of-shared.image" -}}
{{- $root := .root -}}
{{- $service := .service | default dict -}}
{{- $image := $service.image | default dict -}}
{{- $repository := default .name $image.repository -}}
{{- $tag := default $root.Values.image.tag $image.tag -}}
{{- $registry := coalesce $root.Values.global.imageRegistry $root.Values.image.registry "" -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" $registry $repository $tag -}}
{{- else -}}
{{- printf "%s:%s" $repository $tag -}}
{{- end -}}
{{- end -}}

{{/*
Render a Deployment for one service. Caller passes:
  .root      Helm root context
  .name      service name
  .release   Helm release name
  .image     full image reference
  .service   service values map
*/}}
{{- define "of-shared.deployment" -}}
{{- $root := .root -}}
{{- $service := .service -}}
{{- $global := $root.Values.global | default dict -}}
{{- $deploymentFabric := $global.deploymentFabric | default dict -}}
{{- $geo := $deploymentFabric.geoRestrictions | default dict -}}
{{- $platformProfile := $global.platformProfile | default dict -}}
{{- $platformProfileEnabled := true -}}
{{- if hasKey $platformProfile "enabled" -}}
  {{- $platformProfileEnabled = $platformProfile.enabled -}}
{{- end -}}
{{- if hasKey $service "mountPlatformProfile" -}}
  {{- $platformProfileEnabled = $service.mountPlatformProfile -}}
{{- end -}}
{{- $podAnnotations := mergeOverwrite (dict) ($global.podAnnotations | default dict) ($service.podAnnotations | default dict) -}}
{{- $serviceNodeSelector := $service.nodeSelector | default dict -}}
{{- $globalNodeSelector := $geo.requiredNodeLabels | default dict -}}
{{- $nodeSelector := mergeOverwrite (dict) $globalNodeSelector $serviceNodeSelector -}}
{{- $readiness := mergeOverwrite (dict) (($global.probes | default dict).readiness | default dict) (($service.probes | default dict).readiness | default dict) -}}
{{- $liveness := mergeOverwrite (dict) (($global.probes | default dict).liveness | default dict) (($service.probes | default dict).liveness | default dict) -}}
{{- $replicas := default 2 $service.replicas -}}
{{- if hasKey $global "replicaOverride" -}}
  {{- $replicas = $global.replicaOverride -}}
{{- end -}}
{{- $resources := $service.resources | default dict -}}
{{- if hasKey $global "resourcesOverride" -}}
  {{- $resources = $global.resourcesOverride -}}
{{- end -}}
{{- $ports := $service.ports | default (list) -}}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .name }}
  labels:
    {{- include "of-shared.labels" . | nindent 4 }}
spec:
  replicas: {{ $replicas }}
  selector:
    matchLabels:
      app.kubernetes.io/name: {{ .name }}
  template:
    metadata:
      labels:
        {{- include "of-shared.labels" . | nindent 8 }}
      {{- if gt (len $podAnnotations) 0 }}
      annotations:
        {{- toYaml $podAnnotations | nindent 8 }}
      {{- end }}
    spec:
      serviceAccountName: {{ default .name $service.serviceAccountName }}
      {{- with $global.imagePullSecrets }}
      imagePullSecrets:
        {{- range . }}
        - name: {{ . | quote }}
        {{- end }}
      {{- end }}
      terminationGracePeriodSeconds: {{ default ($global.terminationGracePeriodSeconds | default 30) $service.terminationGracePeriodSeconds }}
      securityContext:
        {{- toYaml (default ($global.podSecurityContext | default dict) $service.podSecurityContext) | nindent 8 }}
      {{- if and $global.topologySpreadConstraints $global.topologySpreadConstraints.enabled }}
      topologySpreadConstraints:
        - maxSkew: {{ $global.topologySpreadConstraints.maxSkew | default 1 }}
          topologyKey: {{ $global.topologySpreadConstraints.topologyKey | default "kubernetes.io/hostname" }}
          whenUnsatisfiable: {{ $global.topologySpreadConstraints.whenUnsatisfiable | default "ScheduleAnyway" }}
          labelSelector:
            matchLabels:
              app.kubernetes.io/name: {{ .name }}
      {{- end }}
      {{- if gt (len $nodeSelector) 0 }}
      nodeSelector:
        {{- toYaml $nodeSelector | nindent 8 }}
      {{- end }}
      {{- with $service.affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with $service.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      volumes:
        - name: tmp
          emptyDir: {}
      {{- if $platformProfileEnabled }}
        - name: platform-profile
          configMap:
            name: {{ include "of-shared.platformProfileConfigMapName" (dict "root" $root) }}
      {{- end }}
      {{- with $service.extraVolumes }}
        {{- toYaml . | nindent 8 }}
      {{- end }}
      containers:
        - name: app
          image: {{ .image }}
          imagePullPolicy: {{ $global.imagePullPolicy | default "IfNotPresent" }}
          {{- with $service.command }}
          command:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with $service.args }}
          args:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          ports:
            {{- range ($service.ports | default (list)) }}
            - name: {{ .name }}
              containerPort: {{ .containerPort }}
            {{- end }}
          {{- if or $global.existingSecret $service.envFrom }}
          envFrom:
            {{- if $global.existingSecret }}
            - secretRef:
                name: {{ $global.existingSecret }}
            {{- end }}
            {{- range ($service.envFrom | default (list)) }}
            - {{- toYaml . | nindent 14 | trim }}
            {{- end }}
          {{- end }}
          env:
            {{- if gt (len $ports) 0 }}
            - name: PORT
              value: {{ (toString ((first $ports).containerPort)) | quote }}
            {{- end }}
            - name: OPENFOUNDRY_DEPLOYMENT_CLOUD
              value: {{ $deploymentFabric.cloud | default "kubernetes" | quote }}
            - name: OPENFOUNDRY_DEPLOYMENT_ENVIRONMENT
              value: {{ $deploymentFabric.environment | default "shared" | quote }}
            - name: OPENFOUNDRY_DEPLOYMENT_REGION
              value: {{ $deploymentFabric.region | default "global" | quote }}
            - name: OPENFOUNDRY_DEPLOYMENT_CELL
              value: {{ $deploymentFabric.cell | default "core" | quote }}
            - name: OPENFOUNDRY_ACTIVE_CLOUD_PROVIDER
              value: {{ (($deploymentFabric.multiCloud | default dict).activeProvider | default "") | quote }}
            - name: OPENFOUNDRY_AIRGAPPED
              value: {{ ternary "true" "false" (($deploymentFabric.airGap | default dict).enabled | default false) | quote }}
            - name: OPENFOUNDRY_RELEASE_BUNDLE
              value: {{ (($deploymentFabric.airGap | default dict).releaseBundle | default "") | quote }}
            - name: OPENFOUNDRY_ALLOWED_GEO_COUNTRIES
              value: {{ join "," (($geo.allowedCountries | default (list))) | quote }}
            - name: OPENFOUNDRY_RESIDENCY_LABEL
              value: {{ ($geo.residencyLabel | default "") | quote }}
            {{- if $platformProfileEnabled }}
            - name: OPENFOUNDRY_PLATFORM_PROFILE_PATH
              value: "/etc/openfoundry/platform-profile/deployment-fabric.yaml"
            {{- end }}
            {{- if and $root.Values.apollo $root.Values.apollo.enabled }}
            - name: OPENFOUNDRY_APOLLO_ENABLED
              value: "true"
            - name: OPENFOUNDRY_APOLLO_ACTION_MODE
              value: {{ $root.Values.apollo.actionMode | default "observe" | quote }}
            {{- end }}
            {{- if $global.cassandra.localDc }}
            - name: CASSANDRA_LOCAL_DC
              value: {{ $global.cassandra.localDc | quote }}
            {{- end }}
            {{- if $global.kafka.bootstrap }}
            - name: KAFKA_BOOTSTRAP_SERVERS
              value: {{ $global.kafka.bootstrap | quote }}
            {{- end }}
            {{- if hasKey ($global.kafka | default dict) "topicPrefix" }}
            - name: KAFKA_TOPIC_PREFIX
              value: {{ $global.kafka.topicPrefix | default "" | quote }}
            {{- end }}
            {{- if and (eq .name "identity-federation-service") $global.publicWebOrigin }}
            - name: PUBLIC_WEB_ORIGIN
              value: {{ $global.publicWebOrigin | quote }}
            {{- end }}
            {{- if kindIs "map" ($service.env | default dict) }}
            {{- range $key, $value := ($service.env | default dict) }}
            - name: {{ $key }}
              value: {{ $value | quote }}
            {{- end }}
            {{- else if kindIs "slice" ($service.env | default list) }}
            {{- toYaml ($service.env | default list) | nindent 12 }}
            {{- end }}
            {{- range $key, $ref := ($service.envSecrets | default dict) }}
            - name: {{ $key }}
              valueFrom:
                secretKeyRef:
                  name: {{ $ref.secretName | quote }}
                  key: {{ $ref.key | quote }}
                  {{- if hasKey $ref "optional" }}
                  optional: {{ $ref.optional }}
                  {{- end }}
            {{- end }}
            {{- with $service.extraEnv }}
            {{- toYaml . | nindent 12 }}
            {{- end }}
          volumeMounts:
            - name: tmp
              mountPath: /tmp
          {{- if $platformProfileEnabled }}
            - name: platform-profile
              mountPath: /etc/openfoundry/platform-profile
              readOnly: true
          {{- end }}
          {{- with $service.extraVolumeMounts }}
            {{- toYaml . | nindent 12 }}
          {{- end }}
          securityContext:
            {{- toYaml (default ($global.containerSecurityContext | default dict) $service.containerSecurityContext) | nindent 12 }}
          {{- if and (gt (len $ports) 0) ($readiness.enabled | default true) }}
          readinessProbe:
            httpGet:
              path: {{ $readiness.path | default "/health" }}
              port: {{ $readiness.port | default "http" }}
            initialDelaySeconds: {{ $readiness.initialDelaySeconds | default 8 }}
            periodSeconds: {{ $readiness.periodSeconds | default 10 }}
            timeoutSeconds: {{ $readiness.timeoutSeconds | default 3 }}
            failureThreshold: {{ $readiness.failureThreshold | default 6 }}
          {{- end }}
          {{- if and (gt (len $ports) 0) ($liveness.enabled | default true) }}
          livenessProbe:
            httpGet:
              path: {{ $liveness.path | default "/health" }}
              port: {{ $liveness.port | default "http" }}
            initialDelaySeconds: {{ $liveness.initialDelaySeconds | default 20 }}
            periodSeconds: {{ $liveness.periodSeconds | default 20 }}
            timeoutSeconds: {{ $liveness.timeoutSeconds | default 3 }}
            failureThreshold: {{ $liveness.failureThreshold | default 3 }}
          {{- end }}
          resources:
            {{- toYaml $resources | nindent 12 }}
{{- end }}

{{/* 
Render a Service for one service.
*/}}
{{- define "of-shared.service" -}}
{{- $service := .service -}}
{{- $ports := $service.ports | default (list) -}}
{{- $serviceSpec := $service.service | default dict -}}
{{- if and (gt (len $ports) 0) (not (eq ($serviceSpec.enabled | default true) false)) }}
apiVersion: v1
kind: Service
metadata:
  name: {{ .name }}
  labels:
    {{- include "of-shared.labels" . | nindent 4 }}
  {{- with ($service.serviceAnnotations | default dict) }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  type: {{ $serviceSpec.type | default "ClusterIP" }}
  selector:
    app.kubernetes.io/name: {{ .name }}
  ports:
    {{- range $ports }}
    - name: {{ .name }}
      port: {{ .servicePort | default .containerPort }}
      targetPort: {{ .name }}
    {{- end }}
{{- end }}
{{- end }}

{{/*
Render a default-deny + allow-from-platform NetworkPolicy.
Caller can override `.allowFrom` to a list of pod selectors.
*/}}
{{- define "of-shared.networkpolicy" -}}
{{- $root := .root -}}
{{- $global := $root.Values.global | default dict -}}
{{- $networkPolicy := $global.networkPolicy | default dict -}}
{{/*
  Sprig's `default` returns the fallback when the value is the zero of
  its type — including boolean `false` — so the previous
  `$networkPolicy.enabled | default true` always evaluated to `true`
  when the operator explicitly set `enabled: false` in the profile.
  Use an explicit hasKey check so an explicit `false` is honoured and
  only an unset key falls back to the secure-by-default `true`.
*/}}
{{- $netpolEnabled := true -}}
{{- if hasKey $networkPolicy "enabled" -}}
  {{- $netpolEnabled = $networkPolicy.enabled -}}
{{- end -}}
{{- if $netpolEnabled }}
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: {{ .name }}
  labels:
    {{- include "of-shared.labels" . | nindent 4 }}
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: {{ .name }}
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - podSelector:
            matchLabels:
              openfoundry.io/release: of-platform
        {{- if $networkPolicy.allowSameNamespaceTraffic }}
        - podSelector: {}
        {{- end }}
        {{- range (.allowFrom | default (list)) }}
        - podSelector:
            matchLabels:
              {{- toYaml . | nindent 14 }}
        {{- end }}
  egress:
    - {}
{{- end }}
{{- end }}

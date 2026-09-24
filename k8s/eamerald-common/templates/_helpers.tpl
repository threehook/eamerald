{{- define "eamerald.labels" -}}
app.kubernetes.io/name: eamerald
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
The postgres DSN env var, sourced from an existing Secret. Only rendered when directory.backend is postgres; emits nothing otherwise.
Include inside an `env:` list with `{{ include "eamerald.postgresDsnEnv" . | nindent N }}`.
*/}}
{{- define "eamerald.postgresDsnEnv" -}}
{{- if eq .Values.directory.backend "postgres" -}}
- name: EAMERALD_DIRECTORY_POSTGRES_DSN
  valueFrom:
    secretKeyRef:
      name: {{ required "directory.postgres.existingSecretName is required when directory.backend is postgres" .Values.directory.postgres.existingSecretName }}
      key: dsn
{{- end }}
{{- end -}}

{{/*
Env vars every eameraldd container needs, regardless of role.
*/}}
{{- define "eamerald.commonEnv" -}}
- name: EAMERALD_RUNNING_IN_CONTAINER
  value: "true"
- name: EAMERALD_DB_DIR
  value: /db
- name: EAMERALD_CERTS_DIR
  value: /certs
- name: EAMERALD_DECISIONS_DIR
  value: /decisions
{{- end -}}

{{/*
The entra client secret env vars, only when entra.enabled. Only meaningful for charts that run the directory
(the entra plugin itself is gated the same way in eamerald.configYaml).
*/}}
{{- define "eamerald.entraEnv" -}}
{{- if .Values.entra.enabled }}
- name: ENTRA_TENANT_ID
  value: {{ .Values.entra.tenantId | quote }}
- name: ENTRA_CLIENT_ID
  value: {{ .Values.entra.clientId | quote }}
- name: ENTRA_CLIENT_SECRET
  valueFrom:
    secretKeyRef:
      name: {{ required "entra.existingSecretName is required when entra.enabled is true" .Values.entra.existingSecretName }}
      key: client_secret
{{- end }}
{{- end -}}

{{/*
Container ports every role exposes: health/metrics, plus the directory ports, which are always listened on internally
(the authorizer resolves identity through them) even for charts that don't publish them via their Service.
*/}}
{{- define "eamerald.corePorts" -}}
- name: health
  containerPort: {{ .Values.service.ports.health }}
- name: metrics
  containerPort: {{ .Values.service.ports.metrics }}
- name: dir-grpc
  containerPort: {{ .Values.service.ports.directoryGRPC }}
- name: dir-gateway
  containerPort: {{ .Values.service.ports.directoryGateway }}
{{- end -}}

{{- define "eamerald.consolePorts" -}}
- name: console-grpc
  containerPort: {{ .Values.service.ports.consoleGRPC }}
- name: console-gateway
  containerPort: {{ .Values.service.ports.consoleGateway }}
{{- end -}}

{{- define "eamerald.authorizerPorts" -}}
- name: authz-grpc
  containerPort: {{ .Values.service.ports.authorizerGRPC }}
- name: authz-gateway
  containerPort: {{ .Values.service.ports.authorizerGateway }}
{{- end -}}

{{/*
api.health is a plain (non-TLS) gRPC health service in this config, so Kubernetes' built-in gRPC probe talks to it directly.

readinessProbe can only pin to one named service. Callers pass a context extended with:
  isEdge        - true only for the edge chart: readiness is pinned to the "sync" health check (NOT_SERVING until the
                  first sync from the hub lands), so the pod is excluded from the Service until it actually has data.
  hasAuthorizer - true for charts that run the authorizer.
When git is the policy source and the chart runs an authorizer (standalone only - hub has no authorizer, edge always
pins to "sync" instead), readiness is pinned to git's "git" named health check (NOT_SERVING until the first successful
sync, self-heals to SERVING once one lands) so the pod is excluded from the Service until a policy bundle is loaded.
Liveness intentionally stays on the default health check regardless, so an unsynced repo/hub never causes a restart loop.
*/}}
{{- define "eamerald.readinessProbe" -}}
grpc:
  port: {{ .Values.service.ports.health }}
  {{- if .isEdge }}
  service: "sync"
  {{- else if and .Values.git.enabled .hasAuthorizer }}
  service: "git"
  {{- end }}
initialDelaySeconds: 5
periodSeconds: 10
{{- end -}}

{{- define "eamerald.livenessProbe" -}}
grpc:
  port: {{ .Values.service.ports.health }}
initialDelaySeconds: 10
periodSeconds: 15
{{- end -}}

{{- define "eamerald.volumeMounts" -}}
- name: config
  mountPath: /config
  readOnly: true
- name: certs
  mountPath: /certs
- name: db
  mountPath: /db
- name: decisions
  mountPath: /decisions
{{- end -}}

{{/*
Applying the manifest offline, before eameraldd starts, rather than against the running service: eamerald-db writes to the database file
(or postgres DSN) directly (via an in-process directory server), so this needs no TLS certs, no readiness wait, and no network.
It also guarantees the model exists before the first directory write - the entra sync plugin starts writing users and groups about a second
after eameraldd becomes ready, and those writes are rejected if their object types are not defined.

A boltdb-backed instance's file is single-writer, so this must not run concurrently with eameraldd; an init container is exactly that
guarantee. Postgres has no such constraint - mrld-db init/set are plain idempotent client calls, safe to run on every pod start even
across multiple hub replicas concurrently.

Only used by charts that own a source-of-truth directory to seed (hub, standalone).
*/}}
{{- define "eamerald.manifestInitContainer" -}}
- name: set-manifest
  image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
  imagePullPolicy: {{ .Values.image.pullPolicy }}
  command: ["/bin/sh", "-ec"]
  args:
    {{- if eq .Values.directory.backend "postgres" }}
    - |
      ./mrld-db init "$EAMERALD_DIRECTORY_POSTGRES_DSN"
      ./mrld-db set "$EAMERALD_DIRECTORY_POSTGRES_DSN" /manifest/manifest.yaml
    {{- else }}
    - |
      if [ ! -f /db/directory.db ]; then
        ./mrld-db init /db/directory.db
      fi
      ./mrld-db set /db/directory.db /manifest/manifest.yaml
    {{- end }}
  {{- if eq .Values.directory.backend "postgres" }}
  env:
    {{- include "eamerald.postgresDsnEnv" . | nindent 4 }}
  {{- end }}
  volumeMounts:
    - name: db
      mountPath: /db
    - name: manifest
      mountPath: /manifest
      readOnly: true
{{- end -}}

{{/*
The certs volume: a PVC when persistence is enabled (independent of directory.backend - TLS certs have nothing to do with
the directory store), emptyDir otherwise. Only used by charts that support persistence (hub, standalone); the edge chart
hardcodes emptyDir inline instead, since it never gets a PVC (see eamerald.dbAndCertsPvc).
*/}}
{{- define "eamerald.certsVolume" -}}
{{- if .Values.persistence.enabled }}
- name: certs
  persistentVolumeClaim:
    claimName: {{ .Release.Name }}-certs
{{- else }}
- name: certs
  emptyDir: {}
{{- end }}
{{- end -}}

{{/*
The db volume: a PVC only for a boltdb-backed install with persistence enabled - postgres has nothing local to persist.
Only used by charts that support persistence (hub, standalone); the edge chart hardcodes emptyDir inline instead (its
local cache is disposable by design, rebuilt from the hub on every restart).
*/}}
{{- define "eamerald.dbVolume" -}}
{{- if and .Values.persistence.enabled (ne .Values.directory.backend "postgres") }}
- name: db
  persistentVolumeClaim:
    claimName: {{ .Release.Name }}-db
{{- else }}
- name: db
  emptyDir: {}
{{- end }}
{{- end -}}

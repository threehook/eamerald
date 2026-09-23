{{- define "eamerald.labels" -}}
app.kubernetes.io/name: eamerald
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Roles that run the directory (model/reader/writer/exporter/importer/console):
standalone and hub.
*/}}
{{- define "eamerald.hasDirectory" -}}
{{- if or (eq .Values.role "standalone") (eq .Values.role "hub") -}}true{{- end -}}
{{- end -}}

{{/*
Roles that run the authorizer (and therefore evaluate OPA policy):
standalone and edge.
*/}}
{{- define "eamerald.hasAuthorizer" -}}
{{- if or (eq .Values.role "standalone") (eq .Values.role "edge") -}}true{{- end -}}
{{- end -}}

{{/*
The postgres DSN env var, sourced from an existing Secret. Only rendered when directory.backend is postgres; emits nothing otherwise
Include inside an`env:` list with `{{ include "eamerald.postgresDsnEnv" . | nindent N }}`.
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

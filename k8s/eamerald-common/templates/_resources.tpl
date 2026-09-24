{{/*
The eameraldd config ConfigMap. Callers pass the same extended context as eamerald.configYaml (hasDirectory,
hasAuthorizer, isEdge).
*/}}
{{- define "eamerald.configMap" -}}
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}-config
  labels:
    {{- include "eamerald.labels" . | nindent 4 }}
data:
  config.yaml: |
    {{- include "eamerald.configYaml" . | nindent 4 }}
{{- end -}}

{{/*
The Service exposing whichever ports this role actually runs. Callers pass a context extended with hasDirectory and
hasAuthorizer (health/metrics are always exposed).
*/}}
{{- define "eamerald.service" -}}
apiVersion: v1
kind: Service
metadata:
  name: {{ .Release.Name }}
  labels:
    {{- include "eamerald.labels" . | nindent 4 }}
spec:
  type: {{ .Values.service.type }}
  selector:
    {{- include "eamerald.labels" . | nindent 4 }}
  ports:
    - name: health
      port: {{ .Values.service.ports.health }}
      targetPort: health
    - name: metrics
      port: {{ .Values.service.ports.metrics }}
      targetPort: metrics
  {{- if .hasDirectory }}
    - name: dir-grpc
      port: {{ .Values.service.ports.directoryGRPC }}
      targetPort: dir-grpc
    - name: dir-gateway
      port: {{ .Values.service.ports.directoryGateway }}
      targetPort: dir-gateway
    - name: console-grpc
      port: {{ .Values.service.ports.consoleGRPC }}
      targetPort: console-grpc
    - name: console-gateway
      port: {{ .Values.service.ports.consoleGateway }}
      targetPort: console-gateway
  {{- end }}
  {{- if .hasAuthorizer }}
    - name: authz-grpc
      port: {{ .Values.service.ports.authorizerGRPC }}
      targetPort: authz-grpc
    - name: authz-gateway
      port: {{ .Values.service.ports.authorizerGateway }}
      targetPort: authz-gateway
  {{- end }}
{{- end -}}

{{/*
The directory manifest (object types, relations, permissions) ConfigMap, seeded from directory.manifest.content
(typically --set-file). Only used by charts that own a source-of-truth directory (hub, standalone) - the edge chart
receives its manifest via the aserto_edge sync plugin instead and doesn't call this at all.
*/}}
{{- define "eamerald.manifestConfigMap" -}}
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Release.Name }}-manifest
  labels:
    {{- include "eamerald.labels" . | nindent 4 }}
data:
  manifest.yaml: |
    {{- required "directory.manifest.content is required, e.g. --set-file directory.manifest.content=<path>" .Values.directory.manifest.content | nindent 4 }}
{{- end -}}

{{/*
The db and certs PVCs, for a boltdb-backed install with persistence enabled - i.e. only when replicaCount is expected
to stay 1. The certs PVC is independent of directory.backend (TLS certs have nothing to do with the directory store);
the db PVC additionally requires a non-postgres backend, since postgres has nothing local to persist. Only used by
charts that support persistence (hub, standalone) - the edge chart never gets a PVC (see eamerald.certsVolume /
eamerald.dbVolume, which fall back to emptyDir for it unconditionally).
*/}}
{{- define "eamerald.dbAndCertsPvc" -}}
{{- if .Values.persistence.enabled }}
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: {{ .Release.Name }}-certs
  labels:
    {{- include "eamerald.labels" . | nindent 4 }}
spec:
  accessModes:
    - ReadWriteOnce
  {{- if .Values.persistence.storageClassName }}
  storageClassName: {{ .Values.persistence.storageClassName }}
  {{- end }}
  resources:
    requests:
      storage: {{ .Values.persistence.certs.size }}
{{- if ne .Values.directory.backend "postgres" }}
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: {{ .Release.Name }}-db
  labels:
    {{- include "eamerald.labels" . | nindent 4 }}
spec:
  accessModes:
    - ReadWriteOnce
  {{- if .Values.persistence.storageClassName }}
  storageClassName: {{ .Values.persistence.storageClassName }}
  {{- end }}
  resources:
    requests:
      storage: {{ .Values.persistence.db.size }}
{{- end }}
{{- end }}
{{- end -}}

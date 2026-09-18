{{- define "eamerald.labels" -}}
app.kubernetes.io/name: eamerald
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "topaz.labels" -}}
app.kubernetes.io/name: topaz
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

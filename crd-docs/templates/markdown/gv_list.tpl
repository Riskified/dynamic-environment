{{- define "gvList" -}}
{{- $groupVersions := . -}}

# DynamicEnv CRD Reference

## Packages
{{- range $groupVersions }}
- {{ markdownRenderGVLink . }}
{{- end }}

{{ range $groupVersions }}
{{ template "gvDetails" . }}
{{ end }}

{{- end -}}

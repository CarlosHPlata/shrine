{{- $content := .RawContent -}}
{{- if not (hasPrefix $content "# ") -}}
# {{ .Title }}

{{ end -}}
{{ $content }}

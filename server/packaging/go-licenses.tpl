{{- /* Each Go module in the server's build, for THIRD-PARTY-NOTICES.txt (make notices). */ -}}
{{ range . }}
================================================================================
{{ .Name }} {{ .Version }} ({{ .LicenseName }})
{{ .LicenseURL }}

{{ .LicenseText }}
{{ end }}

{{/*
Expand the name of the chart.
*/}}
{{- define "6musicproxy.fullname" -}}
{{- $name := printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- if not (regexMatch "^[a-z]([-a-z0-9]*[a-z0-9])?$" $name) -}}
{{- $name = printf "proxy-%s" .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- $name | lower -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "6musicproxy.labels" -}}
app.kubernetes.io/name: {{ include "6musicproxy.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Selector labels
*/}}
{{- define "6musicproxy.selectorLabels" -}}
app.kubernetes.io/name: {{ include "6musicproxy.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Name of the chart
*/}}
{{- define "6musicproxy.name" -}}
{{- .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

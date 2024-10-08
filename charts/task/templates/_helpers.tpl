{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "task.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "task.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "task.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "task.labels" -}}
helm.sh/chart: {{ include "task.chart" . }}
{{ include "task.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "task.selectorLabels" -}}
app.kubernetes.io/name: {{ include "task.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
CLI labels
*/}}
{{- define "task.cli.labels" -}}
{{ include "task.labels" . }}
app.kubernetes.io/component: cli
{{- end }}

{{/*
CLI selector labels
*/}}
{{- define "task.cli.selectorLabels" -}}
{{ include "task.selectorLabels" . }}
app.kubernetes.io/component: cli
{{- end }}

{{/*
Server labels
*/}}
{{- define "task.server.labels" -}}
{{ include "task.labels" . }}
app.kubernetes.io/component: server
{{- end }}

{{/*
Server selector labels
*/}}
{{- define "task.server.selectorLabels" -}}
{{ include "task.selectorLabels" . }}
app.kubernetes.io/component: server
{{- end }}

{{/*
Dashboard labels
*/}}
{{- define "task.dashboard.labels" -}}
{{ include "task.labels" . }}
app.kubernetes.io/component: dashboard
{{- end }}

{{/*
Dashboard selector labels
*/}}
{{- define "task.dashboard.selectorLabels" -}}
{{ include "task.selectorLabels" . }}
app.kubernetes.io/component: dashboard
{{- end }}

{{/*
Selector labels (immutable)
*/}}
{{- define "task.immutableSelectorLabels" -}}
app.kubernetes.io/name: {{ include "task.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Server selector labels (immutable)
*/}}
{{- define "task.server.immutableSelectorLabels" -}}
{{ include "task.immutableSelectorLabels" . }}
app.kubernetes.io/component: server
{{- end }}

{{/*
Dashboard selector labels (immutable)
*/}}
{{- define "task.dashboard.immutableSelectorLabels" -}}
{{ include "task.immutableSelectorLabels" . }}
app.kubernetes.io/component: dashboard
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "task.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "task.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
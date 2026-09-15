{{/*
Expand the name of the chart.
*/}}
{{- define "brunernetes.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a fully qualified application name.
*/}}
{{- define "brunernetes.fullname" -}}
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
Chart name and version, as used by the standard chart label.
*/}}
{{- define "brunernetes.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "brunernetes.labels" -}}
helm.sh/chart: {{ include "brunernetes.chart" . }}
{{ include "brunernetes.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/part-of: brunernetes-factory
{{- end }}

{{/*
Selector labels. These must never change for an existing release, because a
Deployment selector is immutable.
*/}}
{{- define "brunernetes.selectorLabels" -}}
app.kubernetes.io/name: {{ include "brunernetes.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Name of the service account used by the controller.
*/}}
{{- define "brunernetes.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "brunernetes.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Container image reference, defaulting the tag to the chart appVersion.
*/}}
{{- define "brunernetes.image" -}}
{{- printf "%s:%s" .Values.image.repository (default .Chart.AppVersion .Values.image.tag) }}
{{- end }}

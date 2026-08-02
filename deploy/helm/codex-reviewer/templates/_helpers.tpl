{{- define "codex-reviewer.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "codex-reviewer.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "codex-reviewer.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
app.kubernetes.io/name: {{ include "codex-reviewer.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "codex-reviewer.selectorLabels" -}}
app.kubernetes.io/name: {{ include "codex-reviewer.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "codex-reviewer.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "codex-reviewer.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- required "serviceAccount.name is required when serviceAccount.create=false" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "codex-reviewer.image" -}}
{{- if .Values.image.fullOverride -}}
{{- .Values.image.fullOverride -}}
{{- else -}}
{{- printf "%s:%s" .Values.image.repository .Values.image.tag -}}
{{- end -}}
{{- end -}}

{{- define "codex-reviewer.reviewerJobImage" -}}
{{- if .Values.reviewerJob.image.fullOverride -}}
{{- .Values.reviewerJob.image.fullOverride -}}
{{- else -}}
{{- printf "%s:%s" .Values.reviewerJob.image.repository .Values.reviewerJob.image.tag -}}
{{- end -}}
{{- end -}}

{{- define "codex-reviewer.sidecarImage" -}}
{{- if .Values.reviewerJob.sidecarImage.fullOverride -}}
{{- .Values.reviewerJob.sidecarImage.fullOverride -}}
{{- else -}}
{{- printf "%s:%s" .Values.reviewerJob.sidecarImage.repository .Values.reviewerJob.sidecarImage.tag -}}
{{- end -}}
{{- end -}}

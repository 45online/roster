{{/*
roster.name keeps the chart name short, useful when concatenated.
*/}}
{{- define "roster.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
roster.fullname is the canonical resource-name prefix. It collapses to
just the chart name when the release name already contains it (so
'helm install roster ./charts/roster' doesn't produce 'roster-roster').
*/}}
{{- define "roster.fullname" -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{/*
Common labels shared by every resource in this chart.
*/}}
{{- define "roster.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{ include "roster.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "roster.selectorLabels" -}}
app.kubernetes.io/name: {{ include "roster.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
roster.serviceAccountName picks the right SA name depending on whether
the chart is creating its own or referencing a pre-existing one.
*/}}
{{- define "roster.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "roster.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
roster.secretName resolves to the Secret holding credentials —
either the user-provided one or the chart-managed one.
*/}}
{{- define "roster.secretName" -}}
{{- if .Values.credentials.existingSecret -}}
{{- .Values.credentials.existingSecret -}}
{{- else -}}
{{- printf "%s-creds" (include "roster.fullname" .) -}}
{{- end -}}
{{- end -}}

{{/*
roster.image renders the full image reference, defaulting tag to
.Chart.AppVersion when not overridden.
*/}}
{{- define "roster.image" -}}
{{- $tag := default .Chart.AppVersion .Values.image.tag -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end -}}

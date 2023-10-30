{{/* vim: set filetype=mustache: */}}

{{/*
Expand the name of the chart.
*/}}
{{- define "dynamic-environment-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "dynamic-environment-operator.fullname" -}}
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

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "dynamic-environment-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Chart name prefix for resource names
Strip the "-operator" suffix from the default .Chart.Name if the nameOverride is not specified.
This enables using a shorter name for the resources, for example dynamic-environment-webhook.
*/}}
{{- define "dynamic-environment-operator.namePrefix" -}}
{{- $defaultNamePrefix := .Chart.Name | trimSuffix "-operator" -}}
{{- default $defaultNamePrefix .Values.nameOverride | trunc 42 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create the name of the webhook service
*/}}
{{- define "dynamic-environment-operator.webhookService" -}}
{{- printf "%s-webhook-service" (include "dynamic-environment-operator.namePrefix" .) -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "dynamic-environment-operator.labels" -}}
helm.sh/chart: {{ include "dynamic-environment-operator.chart" . }}
{{ include "dynamic-environment-operator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Selector labels
*/}}
{{- define "dynamic-environment-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "dynamic-environment-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{/*
Create the name of the service account to use
*/}}
{{- define "dynamic-environment-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
    {{ default (include "dynamic-environment-operator.fullname" .) .Values.serviceAccount.name }}
{{- else -}}
    {{ default "default" .Values.serviceAccount.name }}
{{- end -}}
{{- end -}}

{{/*
Create the name of the webhook cert secret
*/}}
{{- define "dynamic-environment-operator.webhookCertSecret" -}}
{{- printf "%s-tls" (include "dynamic-environment-operator.namePrefix" .) -}}
{{- end -}}

{{/*
Generate certificates for webhook
*/}}
{{- define "dynamic-environment-operator.webhookCerts" -}}
{{- $serviceName := (include "dynamic-environment-operator.webhookService" .) -}}
{{- $secretName := (include "dynamic-environment-operator.webhookCertSecret" .) -}}
{{- $secret := lookup "v1" "Secret" .Release.Namespace $secretName -}}
{{- if (and .Values.webhookTLS.caCert .Values.webhookTLS.cert .Values.webhookTLS.key) -}}
caCert: {{ .Values.webhookTLS.caCert | b64enc }}
clientCert: {{ .Values.webhookTLS.cert | b64enc }}
clientKey: {{ .Values.webhookTLS.key | b64enc }}
{{- else if and .Values.keepTLSSecret $secret -}}
caCert: {{ index $secret.data "ca.crt" }}
clientCert: {{ index $secret.data "tls.crt" }}
clientKey: {{ index $secret.data "tls.key" }}
{{- else -}}
{{- $altNames := list (printf "%s.%s" $serviceName .Release.Namespace) (printf "%s.%s.svc" $serviceName .Release.Namespace) (printf "%s.%s.svc.cluster.local" $serviceName .Release.Namespace) -}}
{{- $ca := genCA "dynamic-environment-operator-ca" 3650 -}}
{{- $cert := genSignedCert (include "dynamic-environment-operator.fullname" .) nil $altNames 3650 $ca -}}
caCert: {{ $ca.Cert | b64enc }}
clientCert: {{ $cert.Cert | b64enc }}
clientKey: {{ $cert.Key | b64enc }}
{{- end -}}
{{- end -}}
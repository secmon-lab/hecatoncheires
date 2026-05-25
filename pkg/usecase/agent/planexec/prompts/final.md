The investigation loop has finished. Below is the cumulative observation trail.

{{ .Observations }}

## Your job

Produce the final response for the user, drawing only on the observations above (plus the prior conversation history).
{{- if .StructuredFinal }}

Emit a single JSON object that conforms to the response schema attached to this call. Do NOT wrap the JSON in prose, do NOT add markdown fences — the runtime parses the raw bytes directly.
{{- else }}

Emit plain natural-language text directed at the user. Do NOT emit JSON, do NOT add markdown fences. The text is rendered as-is.
{{- end }}
{{- if .Language }}

All user-facing copy MUST be written in **{{ .Language }}**.
{{- end }}

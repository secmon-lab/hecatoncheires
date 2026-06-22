The request below was judged simple enough to answer directly, without a separate investigation phase.

# User request

{{ .UserInput }}

## Your job

Answer the request directly. You may call the tools provided to you if — and only if — you need them to answer accurately; otherwise reply from what you already know and the conversation history. Keep it focused: this path exists for straightforward requests, so do not over-investigate.

Emit plain natural-language text directed at the user. Do NOT emit JSON, do NOT add markdown fences. The text is rendered as-is.
{{- if .Language }}

All user-facing copy MUST be written in **{{ .Language }}**.
{{- end }}

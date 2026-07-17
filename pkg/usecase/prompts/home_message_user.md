Output language: {{ .Lang }}
Local date: {{ .Date }}
Time of day: {{ .TimeOfDay }}
Open cases assigned to the user: {{ .OpenCaseLoad }}
Incomplete actions assigned to the user: {{ .DueActionLoad }}
{{ if .WorkspaceNames -}}
Workspaces the user is involved in: {{ range $i, $w := .WorkspaceNames }}{{ if $i }}, {{ end }}{{ $w }}{{ end }}
{{ else -}}
The user is not currently involved in any active case.
{{ end -}}
Tone for this message: {{ .Flavor }} (nonce {{ .Nonce }})
{{ if .RecentMessages -}}
Recently shown messages (do NOT repeat these phrasings):
{{ range .RecentMessages }}- {{ . }}
{{ end -}}
{{ end -}}

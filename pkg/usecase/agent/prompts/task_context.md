The run you are part of is pinned to:
{{- if .WorkspaceID }}
- workspace_id: {{ .WorkspaceID }}
{{- end }}
{{- if .CaseID }}
- case_id: {{ .CaseID }}
{{- end }}
{{- if .SlackChannelID }}
- slack_channel_id: {{ .SlackChannelID }}
{{- end }}
{{- if .SlackThreadTS }}
- slack_thread_ts: {{ .SlackThreadTS }}
{{- end }}
{{- if .CurrentTime }}
- current_time: {{ .CurrentTime }} (UTC)
{{- end }}
{{- if .CurrentTime }}

Resolve every relative date or period in your task against `current_time`
("today", "by tomorrow", "last week", "end of the month"), and report dates in
absolute form.
{{- end }}
{{- if .SlackThreadTS }}

`slack_thread_ts` is the thread this subject's conversation lives in: pass it as a
`slack__get_messages` target (together with `slack_channel_id`) to read that
conversation.
{{- end }}

This case was raised by an emoji reaction on ONE specific Slack message — the anchor, timestamp {{ .AnchorTS }}. Treat that anchored message as the CENTER of the case. Someone reacted to it because it voiced a problem, a complaint, a blocker, or an unmet need — your job is to pin down exactly what that concern is.

The surrounding thread and nearby messages are CONTEXT, not the subject. Read the messages before and after the anchor to work out WHY the problem in the anchor was raised — what led to it, and what it is really about — but keep the case anchored to the reacted message's concern. Do NOT let a tangential point from the thread take over the title or description.

Base the title, the description, and every field on the anchor's problem, explained by the surrounding context.
{{ if .Permalink }}
Include the anchor message's Slack link ({{ .Permalink }}) in the description so the case always points back to its source.
{{ end }}
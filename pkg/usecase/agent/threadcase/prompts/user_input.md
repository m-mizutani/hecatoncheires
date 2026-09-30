{{ if .HasSystemMessages -}}
# Thread so far
{{ range .SystemMessages }}[{{ .Timestamp }}] {{ .Speaker }}: {{ .Text }}
{{ end }}
{{ end -}}
{{ if .HasDeltaMessages -}}
# New messages since last mention
{{ range .DeltaMessages }}[{{ .Timestamp }}] {{ .Speaker }}: {{ .Text }}
{{ end }}
{{ end -}}
{{ if .MentionText -}}
# Current mention
{{ if .MentionSpeaker }}From: {{ .MentionSpeaker }}
{{ end }}{{ .MentionText }}
{{- end -}}
{{ if not (or .HasSystemMessages .HasDeltaMessages .MentionText) -}}
Investigate this case and decide the next action.
{{- end -}}

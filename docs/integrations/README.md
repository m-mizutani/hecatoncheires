# Integrations

Hecatoncheires can wire up external services to surface their content to the AI agent and to feed the Source ingestion pipeline. Each integration is optional: when its credentials are not configured, the server starts normally and the tools that depend on it are left out. Jira is stricter about a partial setup: setting only some of its three flags stops the server from starting (see [Jira](jira.md#2-configure-the-server)).

| Integration | What it enables | Credentials | Tools available to Jobs |
|---|---|---|---|
| [Notion](notion.md) | Agent tools that search and read pages and databases shared with the integration | `--notion-api-token` (`HECATONCHEIRES_NOTION_API_TOKEN`) | Yes |
| [GitHub](github.md) | The GitHub Source pipeline (PR / Issue ingestion) and agent tools that search and read issues, pull requests, files and commits | `--github-app-id`, `--github-app-installation-id`, `--github-app-private-key` | No |
| [Jira](jira.md) | Agent tools that list projects and search and read Jira Cloud issues | `--jira-base-url`, `--jira-email`, `--jira-api-token` | Yes |

Slack is not listed here because it is a required part of a deployment rather than an optional integration; see [Slack Integration](../slack.md).

> **Scope note.** These pages are about *enabling* the Notion, GitHub, and Jira
> services. They are **not** the complete agent-tool list — that lives in
> [Agent Tools](../agent_tools.md). The **Notion** and **Jira** tools are wired
> into the interactive mention / investigation agents **and into unattended
> [Jobs](../configuration.md#job-definitions-job)** (both modes). The **GitHub**
> tools remain interactive / investigation only — they are **not** available to
> Jobs. If you are writing a Job prompt, check
> [Agent Tools → Tools available by context](../agent_tools.md#tools-available-by-context)
> for what a Job can actually call.

## See Also

- [Agent Tools](../agent_tools.md) — the full agent-tool catalogue and the per-context availability matrix (which tools Jobs get vs. the interactive agent).
- [Configuration](../configuration.md) — CLI flags and environment variables, including `--notion-api-token`, the `--github-app-*` flags, and the `--jira-*` flags.
- [Slack](../slack.md) — Slack app setup and authentication, which power the mention agent and assist flow that consume these integration tools.

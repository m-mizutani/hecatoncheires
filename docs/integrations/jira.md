# Jira

Hecatoncheires uses [`github.com/gollem-dev/tools/jira`](https://github.com/gollem-dev/tools) — a read-only Jira Cloud integration — for the agent's Jira tools (list projects, search issues, fetch issues). Unlike Notion and GitHub, Jira does not currently feed the Source ingestion pipeline; it is agent-tools only.

## 1. Generate a Jira API Token

1. Sign in to Jira Cloud with the account whose access you want the agent to use.
2. Open <https://id.atlassian.com/manage-profile/security/api-tokens>.
3. Click **Create API token**, name it (e.g. `hecatoncheires`), and copy the generated token. It is shown only once.

The agent's permissions are exactly the permissions of this account within your Jira site — there is no separate app-level scope to grant.

## 2. Configure the Server

Set all three flags together. When none of them is set, Jira features are disabled and the server runs normally with the other integrations. When only some of them are set, the server refuses to start with `incomplete Jira configuration`, so a typo in one value is not mistaken for an intentional opt-out.

```bash
hecatoncheires serve \
  --jira-base-url=https://your-domain.atlassian.net \
  --jira-email=you@example.com \
  --jira-api-token=your-api-token \
  ...
```

Or via environment variables:

```bash
export HECATONCHEIRES_JIRA_BASE_URL="https://your-domain.atlassian.net"
export HECATONCHEIRES_JIRA_EMAIL="you@example.com"
export HECATONCHEIRES_JIRA_API_TOKEN="your-api-token"
```

When all three are configured, you should see `Jira service enabled` in the server logs at startup. When none is set, the server logs `Jira not configured, Jira agent tools will be disabled` and the Jira-backed agent tools are skipped.

## 3. Agent Tools

When Jira is configured, every agent context (interactive mention agent, assist, investigation sub-agents, and Jobs — see [Agent Tools → Tools available by context](../agent_tools.md#tools-available-by-context)) gains the following gollem tools:

| Tool | Purpose |
| --- | --- |
| `jira_list_projects` | List projects accessible to the account (id, key, name, type, lead), with pagination. |
| `jira_search_issues` | Search issues with JQL syntax; a `project` argument is spliced into the JQL via AND for convenience. Returns key, summary, status, type, assignee, priority, and updated time. |
| `jira_get_issues` | Fetch one or more issues by key/id in a single batch (up to 100); descriptions and optional comments are rendered from Jira's Atlassian Document Format to Markdown. |

Investigation sub-agents (proposal case-draft, thread-mode investigation) only receive these tools when the planner explicitly selects the `jira` ToolSet for a task — see [Agent Tools](../agent_tools.md) for the ToolSet-selection mechanism.

## See Also

- [Integrations](README.md) — every integration, the credentials each needs, and whether Jobs can use its tools.
- [Agent Tools](../agent_tools.md) — the full agent-tool catalogue and the per-context availability matrix.

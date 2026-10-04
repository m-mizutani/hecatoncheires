# GitHub

Hecatoncheires uses a single GitHub App to power both the Source pipeline (PR/Issue ingestion) and the agent's GitHub tools (search, get_issue, get_pull_request, get_file, list_commits). Wiring up the App enables both at once — there is no separate flag for the agent tools.

## GitHub App Setup

1. Create a GitHub App at `https://github.com/settings/apps/new`
2. Grant the following permissions:
   - **Repository permissions**: Issues (Read), Pull Requests (Read), Contents (Read)
3. Install the App on the target organization or repositories
4. Note the App ID, Installation ID, and download the private key

## Configuration

All three flags (`--github-app-id`, `--github-app-installation-id`, `--github-app-private-key`) must be set to enable GitHub features (Source pipeline + agent tools). If any flag is missing, GitHub features are gracefully disabled and the application continues to run normally with other source types.

```bash
hecatoncheires serve \
  --github-app-id=12345 \
  --github-app-installation-id=67890 \
  --github-app-private-key=/path/to/private-key.pem \
  ...
```

The `--github-app-private-key` accepts either a file path to a PEM file or the PEM content directly as a string.

## Source Management

GitHub Sources are managed via the GraphQL API:

- `createGitHubSource` - Create a new GitHub source with repository list
- `updateGitHubSource` - Update an existing GitHub source
- `validateGitHubRepo` - Validate access to a repository before adding it

Repositories can be specified in `owner/repo` format or as full GitHub URLs (e.g., `https://github.com/owner/repo`).

## Agent Tools

When the GitHub App is configured, the Slack mention agent and the assist flow gain the following gollem tools:

| Tool | Purpose |
| --- | --- |
| `github__search` | Search issues and pull requests using GitHub search syntax (`repo:`, `is:open`, `author:`, `label:`, etc.). Up to 50 hits per call. |
| `github__get_issue` | Fetch a single issue (not PR) with its body, labels, and full comment thread. |
| `github__get_pull_request` | Fetch a single PR with body, labels, comments, and reviews. Optional `include_files=true` adds the diff (per-file patches truncated at 20 KB). |
| `github__get_file` | Fetch a file's content at any branch/tag/SHA. UTF-8 text only; binaries return `is_binary=true` with empty content. Capped at 1 MB. |
| `github__list_commits` | List commits with optional `path`, `author`, `since`, `until` filters. Up to 50 commits per call. |

The tools operate within whatever scope the GitHub App's installation grants — there is no per-repository allowlist on the application side.

## See Also

- [Integrations](README.md) — every integration, the credentials each needs, and whether Jobs can use its tools.
- [Agent Tools](../agent_tools.md) — the full agent-tool catalogue and the per-context availability matrix.

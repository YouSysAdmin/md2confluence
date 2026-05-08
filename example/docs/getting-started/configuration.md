# Configuration

The config file lives at `~/.config/md2confluence/config.yaml` by
default, or any path passed with `--config`.

## Fields

| Field | Required | Description |
|---|---|---|
| `email` | yes | Atlassian account email |
| `apiToken` | yes | API token (not your password) |
| `baseUrl` | yes | Confluence wiki base URL |
| `spaces[].key` | yes | Short space key, e.g. `DOCS` |
| `spaces[].path` | yes | Local directory to scan |
| `spaces[].parentTitle` | no | Title of the parent page |
| `spaces[].autoCreateParent` | no | Create the parent page if missing |
| `spaces[].imageWidth` | no | Default `ac:width` for images (px) |

## Environment overrides

The three scalar fields can (and should) come from the environment so
that secrets stay out of the repo:

| Env var | Overrides |
|---|---|
| `MD2CONFLUENCE_EMAIL` | `email` |
| `MD2CONFLUENCE_API_TOKEN` | `apiToken` |
| `MD2CONFLUENCE_BASE_URL` | `baseUrl` |

With that, a committed config file shrinks to just:

```yaml
spaces:
  - key: "DOCS"
    path: "./example/docs"
    parentTitle: "Documentation Home"
    autoCreateParent: true
```

## Setup checklist

- [ ] Create an [API token](https://id.atlassian.com/manage-profile/security/api-tokens)
- [ ] Find your space key (Space settings → Space details → Key)
- [ ] Store credentials in env vars, not the config file
- [ ] Try `md2confluence sync --dry-run` before a live run

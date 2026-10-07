# Tilt Viewer

A VS Code sidebar for a running [Tilt](https://tilt.dev) session. See your resources
and their status, read their logs, and trigger an update — without leaving the editor.

![TiltViewer](.github/images/tilt.gif)

## Install

```sh
make install
```

That builds the extension, packages a `.vsix`, and installs it into VS Code. Reload
the window afterwards.

Override the VS Code binary if you use Insiders:

```sh
make install CODE=code-insiders
```

## Use

1. Run `tilt up` as usual.
2. Open the Tilt icon in the activity bar.

Each resource shows its status, coloured from the theme: green for ok, running or
completed, yellow for building or pending, red for an error.
The inline buttons trigger an update and show the log. A resource with endpoints also
gets a `$(globe)` button that opens one; with several, it asks which.

Click a resource to open its log. One panel serves every resource: clicking another
retitles it and swaps the content, so the editor never fills with log tabs. The
toolbar filters by level (all, info, warnings, errors) and by text — wrap the text in
slashes, as in `/pod-\d+/`, for a regexp. Errors are red and warnings yellow.
`Follow` keeps the view pinned to the newest line. Each resource keeps its last 5000
lines, which a Tilt restart or a reconnect drops; the panel shows the newest 2000 of
them, and `Copy` takes everything the filter leaves.

Resources are grouped by their Tiltfile `labels`, like the Tilt UI: labels A-Z, then
`unlabeled`, then `Tiltfile`. A group header shows its worst status and how many of
its resources are healthy. Without labels the list stays flat.

| Command | What it does |
| --- | --- |
| `Tilt: Connect` / `Tilt: Disconnect` | Open or close the connection. |
| `Tilt: Reconnect` | Drop state and reconnect. Also the status bar item. |
| `Tilt: Show Tilt Logs` | The Tilt-level log, not a resource log. Same panel. |
| `Tilt: Expand All Groups` / `Tilt: Collapse All Groups` | Title-bar buttons on the view. |
| `Tilt: Filter by Status` | Pick any set of statuses. The button fills in while a filter is on; click it to clear. |

## Settings

| Setting | Default | Notes |
| --- | --- | --- |
| `tilt.host` | `localhost` | |
| `tilt.port` | `10350` | |
| `tilt.token` | _empty_ | Session token. Read from `~/.tilt-dev/token` when empty. |
| `tilt.autoConnect` | `true` | Connect when VS Code starts. |
| `tilt.openInEditor` | `true` | `Open in Tilt UI` uses a Simple Browser tab. Off uses the system browser. |

Changing the host, port, or token reconnects.

## How it connects

The extension uses the same web API as the Tilt UI: a WebSocket on `/ws/view` for
resources and logs, and `POST /api/trigger` to trigger an update. Both need Tilt's
session token, which Tilt writes to `~/.tilt-dev/token`.

If the status bar shows `Tilt: disconnected`, Tilt is not listening on the configured
host and port. The extension retries with backoff, so starting `tilt up` later is
enough.

## Demo stack

`demo/Tiltfile` runs a handful of local resources — label groups, every status,
endpoint links, a manual trigger, and coloured output — with no cluster and no
private images. Useful for screenshots. See [demo/README.md](demo/README.md).

## Develop

```sh
make check      # typecheck and unit tests
make build      # bundle to dist/
npm run watch   # rebuild on change
```

Press F5 to launch an Extension Development Host.

## Release

Tag the commit and push the tag:

```sh
git tag v0.1.1 && git push origin v0.1.1
```

`.github/workflows/release.yml` then runs `scripts/package.js`, which syncs
`package.json` to the tag, builds the `.vsix`, publishes it to the VS Code
Marketplace and Open VSX, and attaches it to the GitHub release.

Both publishes are skipped when their token is missing, so a fork releases
nothing by accident. Repository secrets:

| Secret | Used for |
| --- | --- |
| `VSCODE_PUBLISH_TOKEN` | Azure DevOps PAT with Marketplace → Manage. |
| `OPVSX_PUBLISH_TOKEN` | Open VSX access token. Needs `ovsx create-namespace owenrumney` once. |

The marketplace stalls for minutes at a time rather than failing cleanly, so a
publish that times out is retried after 60s and 180s, and the release then polls
the gallery until the new version is indexed.

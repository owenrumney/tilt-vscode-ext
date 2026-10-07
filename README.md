# Tilt Viewer

A VS Code sidebar for a running [Tilt](https://tilt.dev) session. See your resources
and their status, read their logs, and trigger an update — without leaving the editor.

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

Each resource shows its status. Click one to open its log in an Output channel.
The inline buttons trigger an update and show the log.

| Command | What it does |
| --- | --- |
| `Tilt: Connect` / `Tilt: Disconnect` | Open or close the connection. |
| `Tilt: Reconnect` | Drop state and reconnect. Also the status bar item. |
| `Tilt: Show Tilt Logs` | The Tilt-level log, not a resource log. |

## Settings

| Setting | Default | Notes |
| --- | --- | --- |
| `tilt.host` | `localhost` | |
| `tilt.port` | `10350` | |
| `tilt.token` | _empty_ | Session token. Read from `~/.tilt-dev/token` when empty. |
| `tilt.autoConnect` | `true` | Connect when VS Code starts. |

Changing the host, port, or token reconnects.

## How it connects

The extension uses the same web API as the Tilt UI: a WebSocket on `/ws/view` for
resources and logs, and `POST /api/trigger` to trigger an update. Both need Tilt's
session token, which Tilt writes to `~/.tilt-dev/token`.

If the status bar shows `Tilt: disconnected`, Tilt is not listening on the configured
host and port. The extension retries with backoff, so starting `tilt up` later is
enough.

## Develop

```sh
make check      # typecheck and unit tests
make build      # bundle to dist/
npm run watch   # rebuild on change
```

Press F5 to launch an Extension Development Host.

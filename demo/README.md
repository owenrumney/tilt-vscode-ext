# Demo stack

A Tiltfile for screenshots and GIFs. No cluster, no registry, no private images —
every resource is a local command. Needs `python3` on the path for the served sites.

```sh
cd demo && tilt up
```

What it puts on screen:

| Group | Resources | Shows |
| --- | --- | --- |
| `1-frontend` | `web`, `docs` | Served endpoints, one link each. |
| `2-services` | `api`, `worker` | `api` has two links, so the button asks which. `worker` streams colour. |
| `3-infra` | `db-migrate`, `seed-data`, `broken`, `nightly-report` | A job that completes, one that waits on it, a red failure, and a manual trigger. |
| `unlabeled` | `scratch` | A resource with no label. |

`worker` prints a line every two seconds in 16-colour, 256-colour, truecolour, bold
and underline, which is the quickest way to check the ANSI rendering in the log panel.

`nightly-report` starts idle (`auto_init=False`). Press the trigger button to watch a
build run from the tree.

`broken` fails on purpose, so one resource is always red.

For pod names, pod statuses and restart counts, which no local resource can show, use
[`../demo-k8s`](../demo-k8s) instead. It needs a local cluster.

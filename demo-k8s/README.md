# Cluster demo stack

A Tiltfile that deploys into a local Kubernetes cluster, so the extension has real
pod names, pod statuses and restart counts to render. No registry and no image
build: every workload is a public image with its content mounted from a ConfigMap.

For label groups, manual triggers, failures and coloured log output, use
[`../demo`](../demo) — it needs no cluster and is the one to use for screenshots.

```sh
cd demo-k8s && tilt up
```

Needs a local cluster. OrbStack, Docker Desktop, kind, minikube and colima all work;
if `kubectl config current-context` prints something else, add it to the
`allow_k8s_contexts` call at the top of the Tiltfile.

| Group | Resources | Kind | Shows |
| --- | --- | --- | --- |
| `1-frontend` | `web`, `docs` | Deployment | Pod name and status, one port-forward link each. |
| `2-services` | `api` | Deployment | Two replicas and two links, so the link button asks which. |
| `3-infra` | `db-migrate`, `seed-data` | Job | A Job that completes, and one that waits on it. |
| `3-infra` | `flaky` | Deployment | A pod that exits after 15 seconds, so restarts climb. |

Port forwards are offset by 100 from `../demo` (3110, 3111, 3120), so both stacks can
run at once. The second one needs its own UI port: `tilt up --port 10351`.

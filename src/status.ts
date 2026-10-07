import { formatAge, formatDuration, parseTime } from "./time";
import { TargetType, UIResource, UIResourceCondition } from "./types";

export type ResourceStatus =
  | "disabled"
  | "building"
  | "error"
  | "pending"
  | "running"
  | "ok"
  | "none";

const ICONS: Record<ResourceStatus, string> = {
  disabled: "circle-slash",
  building: "sync~spin",
  error: "error",
  pending: "clock",
  running: "pass",
  ok: "pass-filled",
  none: "circle-outline",
};

// Theme colour ids, resolved to a ThemeColor by the tree.
const COLORS: Record<ResourceStatus, string | undefined> = {
  disabled: "disabledForeground",
  building: "charts.yellow",
  error: "charts.red",
  pending: "charts.yellow",
  running: "charts.green",
  ok: "charts.green",
  none: undefined,
};

// Order the status filter offers them in.
export const ALL_STATUSES: ResourceStatus[] = [
  "error",
  "building",
  "pending",
  "running",
  "ok",
  "none",
  "disabled",
];

export const STATUS_LABELS: Record<ResourceStatus, string> = {
  disabled: "Disabled",
  building: "Building",
  error: "Error",
  pending: "Pending",
  running: "Running",
  ok: "OK",
  none: "Unknown",
};

const SEVERITY: ResourceStatus[] = [
  "error",
  "building",
  "pending",
  "none",
  "running",
  "ok",
  "disabled",
];

// A resource's specs list every target, so the deploy type wins over its image.
const TYPE_PRECEDENCE: TargetType[] = [
  "k8s",
  "docker-compose",
  "local",
  "image",
];

// store.HoldReason. Anything unlisted is shown verbatim.
const WAITING_REASONS: Record<string, string> = {
  "waiting-for-dep": "waiting on",
  "waiting-for-deploy": "waiting for deploy",
  "waiting-for-cluster": "waiting for cluster",
  "waiting-for-local": "waiting for a local build",
  "is-unparallelizable-local": "waiting for a local build",
  "waiting-for-uncategorized": "waiting",
  "building-component": "waiting on a build",
  "tiltfile-reload": "reloading Tiltfile",
  reconciling: "reconciling",
};

export function resourceStatus(r: UIResource): ResourceStatus {
  const s = r.status ?? {};
  if (s.disableStatus?.state === "Disabled") {
    return "disabled";
  }
  if (s.updateStatus === "in_progress") {
    return "building";
  }
  if (s.updateStatus === "error" || s.runtimeStatus === "error") {
    return "error";
  }
  if (s.updateStatus === "pending" || s.runtimeStatus === "pending") {
    return "pending";
  }
  if (s.runtimeStatus === "ok" || s.updateStatus === "ok") {
    // A live pod reads as running; a finished Job stays green.
    return s.k8sResourceInfo?.podStatus === "Running" ? "running" : "ok";
  }
  return "none";
}

/** How Tilt deploys the resource, or undefined when it says nothing. */
export function resourceType(r: UIResource): TargetType | undefined {
  const types = (r.status?.specs ?? [])
    .map((s) => s.type)
    .filter((t): t is TargetType => !!t && t !== "unspecified");
  return TYPE_PRECEDENCE.find((t) => types.includes(t));
}

/** True when a change would sync into the running container. */
export function hasLiveUpdate(r: UIResource): boolean {
  return (r.status?.specs ?? []).some((s) => s.hasLiveUpdate);
}

/** The names Tilt is blocked on, in the order it reports them. */
export function waitingOn(r: UIResource): string[] {
  return (r.status?.waiting?.on ?? [])
    .map((ref) => ref.name)
    .filter((name): name is string => !!name);
}

/**
 * Why a build has not started, as a phrase. Tilt reports a reason without the
 * names and names without a reason, so both halves are optional.
 */
export function waitingLabel(r: UIResource): string | undefined {
  const reason = r.status?.waiting?.reason;
  if (!reason) {
    return undefined;
  }
  const phrase = WAITING_REASONS[reason] ?? reason;
  const names = waitingOn(r);
  return names.length ? `${phrase} ${names.join(", ")}` : phrase;
}

/** How long the last finished build took, ignoring one still running. */
export function lastBuildDuration(r: UIResource): string | undefined {
  const build = (r.status?.buildHistory ?? [])[0];
  const start = parseTime(build?.startTime);
  const finish = parseTime(build?.finishTime);
  if (start === undefined || finish === undefined || finish < start) {
    return undefined;
  }
  return formatDuration(finish - start);
}

// When the running version was deployed, relative to now.
function lastDeployAge(
  r: UIResource,
  now: number = Date.now(),
): string | undefined {
  const deployed = parseTime(r.status?.lastDeployTime);
  return deployed === undefined ? undefined : formatAge(deployed, now);
}

/** "44ms, 40m ago" — whichever half Tilt reported. */
export function buildSummary(
  r: UIResource,
  now: number = Date.now(),
): string | undefined {
  const parts = [lastBuildDuration(r), lastDeployAge(r, now)].filter(
    (p): p is string => !!p,
  );
  return parts.length ? parts.join(", ") : undefined;
}

export function condition(
  r: UIResource,
  type: "Ready" | "UpToDate",
): UIResourceCondition | undefined {
  return (r.status?.conditions ?? []).find((c) => c.type === type);
}

/**
 * Why a condition is false, as a phrase. Tilt puts a terse reason and a long
 * message on the same condition, and the reason is the part that fits a line.
 */
export function conditionDetail(
  r: UIResource,
  type: "Ready" | "UpToDate",
): string | undefined {
  const c = condition(r, type);
  if (!c || c.status === "True") {
    return undefined;
  }
  return c.reason || c.message || undefined;
}

/** The kubelet's explanation for the current pod state. */
export function podMessage(r: UIResource): string | undefined {
  return r.status?.k8sResourceInfo?.podStatusMessage || undefined;
}

/** The objects Tilt deploys for this resource, as the Tilt UI names them. */
export function podObjects(r: UIResource): string[] {
  return r.status?.k8sResourceInfo?.displayNames ?? [];
}

export function statusIcon(status: ResourceStatus): string {
  return ICONS[status];
}

export function statusColor(status: ResourceStatus): string | undefined {
  return COLORS[status];
}

/** The endpoints Tilt knows about, in the order the Tilt UI lists them. */
export function resourceLinks(r: UIResource): { name: string; url: string }[] {
  return (r.status?.endpointLinks ?? [])
    .filter((l): l is { name?: string; url: string } => isWebUrl(l.url))
    .map((l) => ({ name: l.name || l.url, url: l.url }));
}

/** A Tiltfile chooses these strings, so only http(s) is handed to the editor. */
export function isWebUrl(url?: string): boolean {
  return /^https?:\/\//i.test(url ?? "");
}

export function filterByStatus(
  resources: UIResource[],
  allowed: ReadonlySet<ResourceStatus>,
): UIResource[] {
  return resources.filter((r) => allowed.has(resourceStatus(r)));
}

// The status a group header shows: the most urgent of its resources.
export function worstStatus(resources: UIResource[]): ResourceStatus {
  const statuses = resources.map(resourceStatus);
  return (
    SEVERITY.find((s) => statuses.includes(s)) ?? (statuses[0] ?? "none")
  );
}

export function statusLabel(r: UIResource): string {
  const s = r.status ?? {};
  switch (resourceStatus(r)) {
    case "disabled":
      return "disabled";
    case "building":
      return "building";
    case "error":
      return lastBuildError(r) ?? "error";
    case "pending":
      // A queued rebuild outranks the old pod's status; otherwise a
      // crash-looping pod is pending to Tilt and the pod status is the answer.
      if (s.queued) {
        return waitingLabel(r) ?? "queued";
      }
      return waitingLabel(r) ?? podLabel(r) ?? "pending";
    case "running":
    case "ok":
      return podLabel(r) ?? "ok";
    default:
      return "";
  }
}

// Build order matches the Tilt UI: status.order, then name.
export function compareResources(a: UIResource, b: UIResource): number {
  const ao = a.status?.order ?? 0;
  const bo = b.status?.order ?? 0;
  if (ao !== bo) {
    return ao - bo;
  }
  return (a.metadata?.name ?? "").localeCompare(b.metadata?.name ?? "");
}

function lastBuildError(r: UIResource): string | undefined {
  const history = r.status?.buildHistory ?? [];
  return history.find((b) => b.error)?.error;
}

// Pod status with its restart count, which Tilt omits while it is zero.
function podLabel(r: UIResource): string | undefined {
  const info = r.status?.k8sResourceInfo;
  if (!info?.podStatus) {
    return undefined;
  }
  const restarts = info.podRestarts ?? 0;
  return restarts ? `${info.podStatus} · ${restarts} restarts` : info.podStatus;
}

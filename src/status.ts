import { UIResource } from "./types";

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
      return s.queued ? "queued" : "pending";
    case "running":
    case "ok":
      return s.k8sResourceInfo?.podStatus ?? "ok";
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

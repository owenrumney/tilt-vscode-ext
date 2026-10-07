import { UIResource } from "./types";

export type ResourceStatus =
  | "disabled"
  | "building"
  | "error"
  | "pending"
  | "ok"
  | "none";

const ICONS: Record<ResourceStatus, string> = {
  disabled: "circle-slash",
  building: "sync~spin",
  error: "error",
  pending: "clock",
  ok: "pass",
  none: "circle-outline",
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
    return "ok";
  }
  return "none";
}

export function statusIcon(status: ResourceStatus): string {
  return ICONS[status];
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

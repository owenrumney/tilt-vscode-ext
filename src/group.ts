import { UIResource } from "./types";

export const UNLABELED = "unlabeled";
export const TILTFILE = "Tiltfile";

const TILTFILE_RESOURCE = "(Tiltfile)";

export interface ResourceGroup {
  label: string;
  resources: UIResource[];
}

/**
 * Group names are label values, as in the Tilt UI. Keys with a prefix belong to
 * external tooling, so they are ignored.
 */
export function resourceLabels(r: UIResource): string[] {
  const labels = r.metadata?.labels ?? {};
  return Object.keys(labels)
    .filter((key) => !key.includes("/"))
    .map((key) => labels[key])
    .filter((value): value is string => !!value);
}

/**
 * Splits resources into the groups the Tilt sidebar shows: labels A-Z, then
 * unlabeled, then the Tiltfile. Returns [] when nothing is labelled, meaning
 * the caller should show a flat list.
 */
export function groupResources(resources: UIResource[]): ResourceGroup[] {
  const byLabel = new Map<string, UIResource[]>();
  const unlabeled: UIResource[] = [];
  const tiltfile: UIResource[] = [];

  for (const r of resources) {
    const labels = resourceLabels(r);
    if (labels.length === 0) {
      (r.metadata?.name === TILTFILE_RESOURCE ? tiltfile : unlabeled).push(r);
      continue;
    }
    for (const label of labels) {
      const existing = byLabel.get(label);
      if (existing) {
        existing.push(r);
      } else {
        byLabel.set(label, [r]);
      }
    }
  }

  if (byLabel.size === 0) {
    return [];
  }

  const groups = [...byLabel.keys()]
    .sort((a, b) => a.localeCompare(b))
    .map((label) => ({ label, resources: byLabel.get(label) ?? [] }));
  if (unlabeled.length) {
    groups.push({ label: UNLABELED, resources: unlabeled });
  }
  if (tiltfile.length) {
    groups.push({ label: TILTFILE, resources: tiltfile });
  }
  return groups;
}

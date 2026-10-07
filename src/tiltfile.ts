import * as path from "path";

/** A Tiltfile the extension can run `tilt up` against. */
export interface TiltfileEntry {
  /** Absolute path of the Tiltfile itself. */
  file: string;
  /** Directory to run tilt in. */
  dir: string;
  /** What the quick pick shows: the path relative to its workspace folder. */
  label: string;
}

/**
 * Orders discovered Tiltfiles so the one at a workspace root comes first, then
 * the shallowest, then alphabetically. The first entry is what a toolbar button
 * with no prompt would use.
 */
export function describeTiltfiles(
  files: string[],
  roots: string[],
): TiltfileEntry[] {
  return files
    .map((file) => entry(file, roots))
    .sort((a, b) => {
      const depthDiff = depth(a.label) - depth(b.label);
      return depthDiff !== 0 ? depthDiff : a.label.localeCompare(b.label);
    });
}

/** The command line for `tilt up`, honouring a non-default UI port. */
export function upArgs(port: number): string[] {
  return port === DEFAULT_PORT ? ["up"] : ["up", "--port", String(port)];
}

/** `tilt down` takes no port: it talks to the cluster, not the UI. */
export function downArgs(): string[] {
  return ["down"];
}

export const DEFAULT_PORT = 10350;

function entry(file: string, roots: string[]): TiltfileEntry {
  const dir = path.dirname(file);
  // The longest matching root wins, so a nested workspace folder labels its
  // own files rather than the outer folder doing it.
  const root = roots
    .filter((r) => file === r || file.startsWith(r + path.sep))
    .sort((a, b) => b.length - a.length)[0];
  const label = root ? path.relative(root, file) : file;
  return { file, dir, label: label || path.basename(file) };
}

function depth(label: string): number {
  return label.split(path.sep).length;
}

/**
 * The Tiltfile a running Tilt was started with, from its engine dump.
 *
 * Tilt reports an absolute path in `DesiredTiltfilePath`, which is how the
 * down button can target the session that is actually running rather than
 * asking again — including a session someone started outside the editor.
 */
export function tiltfilePathFromEngineDump(dump: unknown): string | undefined {
  if (!dump || typeof dump !== "object") {
    return undefined;
  }
  const path = (dump as { DesiredTiltfilePath?: unknown }).DesiredTiltfilePath;
  return typeof path === "string" && path.trim() ? path : undefined;
}

// Tilt sends the zero time rather than omitting an unset timestamp.
const ZERO_TIME = "0001-01-01T00:00:00Z";

/** Milliseconds since the epoch, or undefined when Tilt means "never". */
export function parseTime(iso?: string): number | undefined {
  if (!iso || iso === ZERO_TIME) {
    return undefined;
  }
  const ms = Date.parse(iso);
  return Number.isNaN(ms) ? undefined : ms;
}

/** A build duration, at the precision a build of that length deserves. */
export function formatDuration(ms: number): string {
  if (ms < 1000) {
    return `${Math.round(ms)}ms`;
  }
  if (ms < 60_000) {
    return `${(ms / 1000).toFixed(1)}s`;
  }
  const minutes = Math.floor(ms / 60_000);
  const seconds = Math.round((ms % 60_000) / 1000);
  return seconds ? `${minutes}m ${seconds}s` : `${minutes}m`;
}

/** How long ago, in the coarsest unit that still says something. */
export function formatAge(ms: number, now: number): string {
  const elapsed = Math.max(0, now - ms);
  if (elapsed < 10_000) {
    return "just now";
  }
  if (elapsed < 60_000) {
    return `${Math.floor(elapsed / 1000)}s ago`;
  }
  if (elapsed < 3_600_000) {
    return `${Math.floor(elapsed / 60_000)}m ago`;
  }
  if (elapsed < 86_400_000) {
    return `${Math.floor(elapsed / 3_600_000)}h ago`;
  }
  return `${Math.floor(elapsed / 86_400_000)}d ago`;
}

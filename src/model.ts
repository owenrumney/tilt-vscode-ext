import { compareResources } from "./status";
import { LogSegment, UIResource, View } from "./types";

export interface RoutedSegment {
  /** Resource name, or undefined for Tilt-level logs. */
  resource?: string;
  text: string;
  level?: string;
}

export interface MergeResult {
  /** True when the resource set changed and the tree needs a refresh. */
  resourcesChanged: boolean;
  /** True when tiltStartTime moved, meaning Tilt restarted. */
  restarted: boolean;
  segments: RoutedSegment[];
}

/**
 * Accumulates the deltas that /ws/view streams. The first message is complete;
 * later ones carry only what changed, so state must persist between them.
 */
export class ViewModel {
  private resources = new Map<string, UIResource>();
  private spans = new Map<string, string | undefined>();
  private tiltStartTime?: string;

  merge(view: View): MergeResult {
    const restarted =
      this.tiltStartTime !== undefined &&
      view.tiltStartTime !== undefined &&
      view.tiltStartTime !== this.tiltStartTime;

    if (restarted) {
      this.clear();
    }
    if (view.tiltStartTime) {
      this.tiltStartTime = view.tiltStartTime;
    }

    let resourcesChanged = restarted;
    for (const r of view.uiResources ?? []) {
      const name = r.metadata?.name;
      if (!name) {
        continue;
      }
      if (r.metadata?.deletionTimestamp) {
        resourcesChanged = this.resources.delete(name) || resourcesChanged;
      } else {
        this.resources.set(name, r);
        resourcesChanged = true;
      }
    }

    for (const [id, span] of Object.entries(view.logList?.spans ?? {})) {
      this.spans.set(id, span?.manifestName || undefined);
    }

    const segments = (view.logList?.segments ?? [])
      .filter((s): s is LogSegment & { text: string } => !!s.text)
      .map((s) => ({
        resource: s.spanId ? this.spans.get(s.spanId) : undefined,
        text: s.text,
        level: s.level,
      }));

    return { resourcesChanged, restarted, segments };
  }

  clear(): void {
    this.resources.clear();
    this.spans.clear();
    this.tiltStartTime = undefined;
  }

  list(): UIResource[] {
    return [...this.resources.values()].sort(compareResources);
  }

  get(name: string): UIResource | undefined {
    return this.resources.get(name);
  }
}

// Subset of Tilt's webview.View that this extension reads.
// See https://api.tilt.dev/interface/ui-resource-v1alpha1.html

export type RuntimeStatus =
  | "unknown"
  | "ok"
  | "pending"
  | "error"
  | "not_applicable"
  | "none";

export type UpdateStatus =
  | "none"
  | "in_progress"
  | "ok"
  | "pending"
  | "error"
  | "not_applicable";

export interface UIResourceLink {
  name?: string;
  url?: string;
}

export interface UIBuildTerminated {
  startTime?: string;
  finishTime?: string;
  error?: string;
  warnings?: string[];
  spanID?: string;
}

export interface UIResource {
  metadata?: {
    name?: string;
    deletionTimestamp?: string;
  };
  status?: {
    runtimeStatus?: RuntimeStatus;
    updateStatus?: UpdateStatus;
    endpointLinks?: UIResourceLink[];
    buildHistory?: UIBuildTerminated[];
    currentBuild?: { startTime?: string; spanID?: string };
    hasPendingChanges?: boolean;
    queued?: boolean;
    order?: number;
    disableStatus?: { state?: string };
    k8sResourceInfo?: { podName?: string; podStatus?: string; podRestarts?: number };
  };
}

export interface LogSpan {
  manifestName?: string;
}

export interface LogSegment {
  spanId?: string;
  time?: string;
  text?: string;
  level?: string;
}

export interface LogList {
  spans?: Record<string, LogSpan>;
  segments?: LogSegment[];
  fromCheckpoint?: number;
  toCheckpoint?: number;
}

export interface View {
  uiResources?: UIResource[];
  logList?: LogList;
  tiltStartTime?: string;
  fatalError?: string;
}

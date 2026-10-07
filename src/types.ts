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

/** api.UIResourceTargetType. "image" appears alongside the deploy type. */
export type TargetType =
  | "unspecified"
  | "image"
  | "k8s"
  | "docker-compose"
  | "local";

export interface UIResourceTargetSpec {
  id?: string;
  type?: TargetType;
  hasLiveUpdate?: boolean;
}

export interface UIResourceStateWaitingOnRef {
  kind?: string;
  name?: string;
}

/** Tilt's store.HoldReason, the reason a build has not started yet. */
export interface UIResourceStateWaiting {
  reason?: string;
  on?: UIResourceStateWaitingOnRef[];
}

export interface UIBuildTerminated {
  startTime?: string;
  finishTime?: string;
  error?: string;
  warnings?: string[];
  spanID?: string;
}

/** api.UIResourceCondition. Only these two types are reported. */
export interface UIResourceCondition {
  type?: "Ready" | "UpToDate";
  status?: "True" | "False" | "Unknown";
  lastTransitionTime?: string;
  reason?: string;
  message?: string;
}

export interface UIResourceKubernetes {
  podName?: string;
  podStatus?: string;
  podStatusMessage?: string;
  podRestarts?: number;
  displayNames?: string[];
}

export interface UIResource {
  metadata?: {
    name?: string;
    deletionTimestamp?: string;
    labels?: Record<string, string>;
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
    k8sResourceInfo?: UIResourceKubernetes;
    specs?: UIResourceTargetSpec[];
    waiting?: UIResourceStateWaiting;
    conditions?: UIResourceCondition[];
    lastDeployTime?: string;
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

import assert from "node:assert/strict";
import { test } from "node:test";
import {
  ALL_STATUSES,
  ResourceStatus,
  compareResources,
  filterByStatus,
  hasLiveUpdate,
  resourceLinks,
  resourceStatus,
  resourceType,
  statusColor,
  statusIcon,
  statusLabel,
  buildSummary,
  conditionDetail,
  lastBuildDuration,
  podMessage,
  podObjects,
  waitingLabel,
  waitingOn,
  worstStatus,
} from "../src/status";
import { UIResource } from "../src/types";

function res(status: UIResource["status"], name = "r"): UIResource {
  return { metadata: { name }, status };
}

test("resourceStatus", () => {
  const cases: [string, UIResource["status"], string][] = [
    ["disabled beats everything", { disableStatus: { state: "Disabled" }, updateStatus: "error" }, "disabled"],
    ["in_progress is building", { updateStatus: "in_progress", runtimeStatus: "error" }, "building"],
    ["update error", { updateStatus: "error", runtimeStatus: "ok" }, "error"],
    ["runtime error", { updateStatus: "ok", runtimeStatus: "error" }, "error"],
    ["error beats pending", { updateStatus: "pending", runtimeStatus: "error" }, "error"],
    ["update pending", { updateStatus: "pending", runtimeStatus: "none" }, "pending"],
    ["runtime pending", { updateStatus: "ok", runtimeStatus: "pending" }, "pending"],
    ["runtime ok", { runtimeStatus: "ok" }, "ok"],
    ["a live pod is running", { runtimeStatus: "ok", k8sResourceInfo: { podStatus: "Running" } }, "running"],
    ["a finished job stays ok", { runtimeStatus: "ok", k8sResourceInfo: { podStatus: "Completed" } }, "ok"],
    ["local resource, update ok only", { updateStatus: "ok", runtimeStatus: "not_applicable" }, "ok"],
    ["nothing known", {}, "none"],
    ["missing status", undefined, "none"],
  ];
  for (const [name, status, want] of cases) {
    assert.equal(resourceStatus(res(status)), want, name);
  }
});

test("every status has an icon", () => {
  for (const s of ["disabled", "building", "error", "pending", "running", "ok", "none"] as const) {
    assert.ok(statusIcon(s), s);
  }
});

test("resourceLinks drops empty urls and falls back to the url as a name", () => {
  const r = res({
    endpointLinks: [
      { name: "api", url: "http://localhost:3000" },
      { name: "no url" },
      { url: "http://localhost:9000" },
    ],
  });
  assert.deepEqual(resourceLinks(r), [
    { name: "api", url: "http://localhost:3000" },
    { name: "http://localhost:9000", url: "http://localhost:9000" },
  ]);
  assert.deepEqual(resourceLinks(res({})), []);
});

test("resourceLinks drops non-http schemes", () => {
  const r = res({
    endpointLinks: [
      { name: "evil", url: "vscode://ms-vscode.remote-server/x" },
      { name: "file", url: "file:///etc/passwd" },
      { name: "ok", url: "https://localhost:3000" },
    ],
  });
  assert.deepEqual(resourceLinks(r), [
    { name: "ok", url: "https://localhost:3000" },
  ]);
});

test("filterByStatus keeps only the chosen statuses", () => {
  const resources = [
    res({ updateStatus: "error" }, "bad"),
    res({ runtimeStatus: "ok" }, "good"),
    res({ updateStatus: "in_progress" }, "busy"),
  ];
  const kept = filterByStatus(resources, new Set<ResourceStatus>(["error", "building"]));
  assert.deepEqual(
    kept.map((r) => r.metadata?.name),
    ["bad", "busy"],
  );
  assert.deepEqual(filterByStatus(resources, new Set(ALL_STATUSES)), resources);
});

test("statusColor maps the states to theme colours", () => {
  assert.equal(statusColor("ok"), "charts.green");
  assert.equal(statusColor("running"), "charts.green");
  assert.equal(statusColor("pending"), "charts.yellow");
  assert.equal(statusColor("building"), "charts.yellow");
  assert.equal(statusColor("error"), "charts.red");
  assert.equal(statusColor("none"), undefined);
});

test("statusLabel surfaces the build error", () => {
  const r = res({
    updateStatus: "error",
    buildHistory: [{ error: "exit status 1" }, { error: "older" }],
  });
  assert.equal(statusLabel(r), "exit status 1");
});

test("statusLabel falls back to error when history is empty", () => {
  assert.equal(statusLabel(res({ updateStatus: "error" })), "error");
});

test("statusLabel shows pod status when ok", () => {
  const r = res({ runtimeStatus: "ok", k8sResourceInfo: { podStatus: "Running" } });
  assert.equal(statusLabel(r), "Running");
});

test("statusLabel distinguishes queued from pending", () => {
  assert.equal(statusLabel(res({ updateStatus: "pending", queued: true })), "queued");
  assert.equal(statusLabel(res({ updateStatus: "pending" })), "pending");
});

test("statusLabel prefers queued over the old pod status", () => {
  const r = res({
    updateStatus: "pending",
    queued: true,
    k8sResourceInfo: { podStatus: "Running" },
  });
  assert.equal(statusLabel(r), "queued");
});

test("statusLabel shows the pod status of a pending resource that is not queued", () => {
  const r = res({
    updateStatus: "pending",
    k8sResourceInfo: { podStatus: "CrashLoopBackOff" },
  });
  assert.equal(statusLabel(r), "CrashLoopBackOff");
});

test("statusLabel prefers the waiting reason over queued", () => {
  const r = res({
    updateStatus: "pending",
    queued: true,
    waiting: { reason: "waiting-for-dep", on: [{ kind: "UIResource", name: "db-migrate" }] },
  });
  assert.equal(statusLabel(r), "waiting on db-migrate");
});

test("resourceType prefers the deploy target over its image", () => {
  const cases: [string, UIResource["status"], string | undefined][] = [
    ["k8s beats image", { specs: [{ type: "image" }, { type: "k8s" }] }, "k8s"],
    ["local", { specs: [{ type: "local" }] }, "local"],
    ["compose", { specs: [{ type: "docker-compose" }, { type: "image" }] }, "docker-compose"],
    ["image alone", { specs: [{ type: "image" }] }, "image"],
    ["unspecified is not a type", { specs: [{ type: "unspecified" }] }, undefined],
    ["no specs", {}, undefined],
    ["missing status", undefined, undefined],
  ];
  for (const [name, status, want] of cases) {
    assert.equal(resourceType(res(status)), want, name);
  }
});

test("hasLiveUpdate is true when any target syncs", () => {
  assert.equal(hasLiveUpdate(res({ specs: [{ type: "image", hasLiveUpdate: true }, { type: "k8s" }] })), true);
  assert.equal(hasLiveUpdate(res({ specs: [{ type: "k8s" }] })), false);
  assert.equal(hasLiveUpdate(res({})), false);
});

test("waitingOn lists the named refs only", () => {
  const r = res({
    waiting: { reason: "waiting-for-dep", on: [{ name: "a" }, { kind: "UIResource" }, { name: "b" }] },
  });
  assert.deepEqual(waitingOn(r), ["a", "b"]);
  assert.deepEqual(waitingOn(res({})), []);
});

test("waitingLabel reads Tilt's hold reasons", () => {
  const cases: [string, UIResource["status"], string | undefined][] = [
    ["dep with a name", { waiting: { reason: "waiting-for-dep", on: [{ name: "db" }] } }, "waiting on db"],
    ["two names", { waiting: { reason: "waiting-for-dep", on: [{ name: "a" }, { name: "b" }] } }, "waiting on a, b"],
    ["reason with no names", { waiting: { reason: "waiting-for-cluster" } }, "waiting for cluster"],
    ["unknown reason passes through", { waiting: { reason: "brand-new" } }, "brand-new"],
    ["names with no reason", { waiting: { on: [{ name: "db" }] } }, undefined],
    ["no waiting", {}, undefined],
  ];
  for (const [name, status, want] of cases) {
    assert.equal(waitingLabel(res(status)), want, name);
  }
});

test("compareResources sorts by order then name", () => {
  const sorted = [
    res({ order: 2 }, "beta"),
    res({ order: 1 }, "zebra"),
    res({ order: 2 }, "alpha"),
    res(undefined, "tiltfile"),
  ].sort(compareResources);
  assert.deepEqual(
    sorted.map((r) => r.metadata?.name),
    ["tiltfile", "zebra", "alpha", "beta"],
  );
});

test("worstStatus picks the most urgent child", () => {
  const cases: [string, UIResource["status"][], string][] = [
    ["error wins", [{ runtimeStatus: "ok" }, { updateStatus: "error" }], "error"],
    ["building beats pending", [{ updateStatus: "pending" }, { updateStatus: "in_progress" }], "building"],
    ["pending beats none", [{}, { updateStatus: "pending" }], "pending"],
    ["none beats ok", [{ runtimeStatus: "ok" }, {}], "none"],
    ["all ok", [{ runtimeStatus: "ok" }], "ok"],
    ["empty group", [], "none"],
  ];
  for (const [name, statuses, want] of cases) {
    assert.equal(worstStatus(statuses.map((s) => res(s))), want, name);
  }
});

test("statusLabel shows a crash-looping pod rather than pending", () => {
  const r = res({
    updateStatus: "ok",
    runtimeStatus: "pending",
    k8sResourceInfo: { podStatus: "Error", podRestarts: 12 },
  });
  assert.equal(statusLabel(r), "Error · 12 restarts");
});

test("statusLabel omits a zero restart count", () => {
  const r = res({ runtimeStatus: "ok", k8sResourceInfo: { podStatus: "Running" } });
  assert.equal(statusLabel(r), "Running");
});

const NOW = Date.parse("2026-10-07T18:00:00Z");

test("lastBuildDuration reads the most recent build", () => {
  const r = res({
    buildHistory: [
      { startTime: "2026-10-07T17:22:53.420767Z", finishTime: "2026-10-07T17:22:53.464555Z" },
      { startTime: "2026-10-07T17:00:00Z", finishTime: "2026-10-07T17:00:09Z" },
    ],
  });
  assert.equal(lastBuildDuration(r), "44ms");
});

test("lastBuildDuration ignores an unfinished or inverted build", () => {
  assert.equal(lastBuildDuration(res({ buildHistory: [{ startTime: "2026-10-07T17:00:00Z" }] })), undefined);
  assert.equal(
    lastBuildDuration(
      res({ buildHistory: [{ startTime: "2026-10-07T17:00:09Z", finishTime: "2026-10-07T17:00:00Z" }] }),
    ),
    undefined,
  );
  assert.equal(lastBuildDuration(res({})), undefined);
});

test("buildSummary joins whichever halves Tilt reported", () => {
  const both = res({
    buildHistory: [{ startTime: "2026-10-07T17:22:53.420767Z", finishTime: "2026-10-07T17:22:53.464555Z" }],
    lastDeployTime: "2026-10-07T17:20:00Z",
  });
  assert.equal(buildSummary(both, NOW), "44ms, 40m ago");
  assert.equal(buildSummary(res({ lastDeployTime: "2026-10-07T17:20:00Z" }), NOW), "40m ago");
  assert.equal(buildSummary(res({ lastDeployTime: "0001-01-01T00:00:00Z" }), NOW), undefined);
  assert.equal(buildSummary(res({}), NOW), undefined);
});

test("conditionDetail only speaks when a condition is false", () => {
  const r = res({
    conditions: [
      { type: "UpToDate", status: "True" },
      { type: "Ready", status: "False", reason: "RuntimePending" },
    ],
  });
  assert.equal(conditionDetail(r, "Ready"), "RuntimePending");
  assert.equal(conditionDetail(r, "UpToDate"), undefined);
  assert.equal(conditionDetail(res({}), "Ready"), undefined);
});

test("conditionDetail falls back to the message when there is no reason", () => {
  const r = res({ conditions: [{ type: "Ready", status: "False", message: "pod never started" }] });
  assert.equal(conditionDetail(r, "Ready"), "pod never started");
});

test("podMessage and podObjects read the kubernetes info", () => {
  const r = res({
    k8sResourceInfo: {
      podStatusMessage: "back-off 5m0s restarting failed container=flaky",
      displayNames: ["web:service", "web:deployment"],
    },
  });
  assert.equal(podMessage(r), "back-off 5m0s restarting failed container=flaky");
  assert.deepEqual(podObjects(r), ["web:service", "web:deployment"]);
  assert.equal(podMessage(res({ k8sResourceInfo: { podStatusMessage: "" } })), undefined);
  assert.deepEqual(podObjects(res({})), []);
});

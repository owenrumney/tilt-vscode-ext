import assert from "node:assert/strict";
import { test } from "node:test";
import {
  ALL_STATUSES,
  ResourceStatus,
  compareResources,
  filterByStatus,
  resourceLinks,
  resourceStatus,
  statusColor,
  statusIcon,
  statusLabel,
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

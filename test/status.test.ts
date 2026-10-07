import assert from "node:assert/strict";
import { test } from "node:test";
import {
  compareResources,
  resourceStatus,
  statusIcon,
  statusLabel,
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
    ["local resource, update ok only", { updateStatus: "ok", runtimeStatus: "not_applicable" }, "ok"],
    ["nothing known", {}, "none"],
    ["missing status", undefined, "none"],
  ];
  for (const [name, status, want] of cases) {
    assert.equal(resourceStatus(res(status)), want, name);
  }
});

test("every status has an icon", () => {
  for (const s of ["disabled", "building", "error", "pending", "ok", "none"] as const) {
    assert.ok(statusIcon(s), s);
  }
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

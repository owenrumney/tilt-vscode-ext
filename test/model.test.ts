import assert from "node:assert/strict";
import { test } from "node:test";
import { ViewModel } from "../src/model";

test("first message populates resources", () => {
  const m = new ViewModel();
  const r = m.merge({
    tiltStartTime: "t0",
    uiResources: [
      { metadata: { name: "api" }, status: { order: 1 } },
      { metadata: { name: "web" }, status: { order: 2 } },
    ],
  });
  assert.equal(r.resourcesChanged, true);
  assert.equal(r.restarted, false);
  assert.deepEqual(m.list().map((x) => x.metadata?.name), ["api", "web"]);
});

test("delta updates one resource and keeps the rest", () => {
  const m = new ViewModel();
  m.merge({
    tiltStartTime: "t0",
    uiResources: [
      { metadata: { name: "api" }, status: { order: 1, updateStatus: "ok" } },
      { metadata: { name: "web" }, status: { order: 2, updateStatus: "ok" } },
    ],
  });
  m.merge({
    tiltStartTime: "t0",
    uiResources: [{ metadata: { name: "api" }, status: { order: 1, updateStatus: "in_progress" } }],
  });
  assert.equal(m.list().length, 2);
  assert.equal(m.get("api")?.status?.updateStatus, "in_progress");
  assert.equal(m.get("web")?.status?.updateStatus, "ok");
});

test("deletionTimestamp removes the resource", () => {
  const m = new ViewModel();
  m.merge({ uiResources: [{ metadata: { name: "api" } }] });
  const r = m.merge({
    uiResources: [{ metadata: { name: "api", deletionTimestamp: "2026-01-01T00:00:00Z" } }],
  });
  assert.equal(r.resourcesChanged, true);
  assert.deepEqual(m.list(), []);
});

test("deleting an unknown resource is not a change", () => {
  const m = new ViewModel();
  const r = m.merge({
    uiResources: [{ metadata: { name: "gone", deletionTimestamp: "2026-01-01T00:00:00Z" } }],
  });
  assert.equal(r.resourcesChanged, false);
});

test("segments route to the resource named by their span", () => {
  const m = new ViewModel();
  const r = m.merge({
    logList: {
      spans: { "build:1": { manifestName: "api" }, "": {} },
      segments: [
        { spanId: "build:1", text: "building api\n" },
        { spanId: "", text: "tilt level\n" },
      ],
    },
  });
  assert.deepEqual(r.segments, [
    { resource: "api", text: "building api\n", level: undefined },
    { resource: undefined, text: "tilt level\n", level: undefined },
  ]);
});

test("spans declared in an earlier message still route", () => {
  const m = new ViewModel();
  m.merge({ logList: { spans: { "pod:1": { manifestName: "web" } }, segments: [] } });
  const r = m.merge({ logList: { segments: [{ spanId: "pod:1", text: "hello\n" }] } });
  assert.deepEqual(r.segments, [{ resource: "web", text: "hello\n", level: undefined }]);
});

test("a segment with an unknown span goes to the Tilt log", () => {
  const m = new ViewModel();
  const r = m.merge({ logList: { segments: [{ spanId: "mystery", text: "x\n" }] } });
  assert.equal(r.segments[0].resource, undefined);
});

test("empty segments are dropped", () => {
  const m = new ViewModel();
  const r = m.merge({ logList: { segments: [{ spanId: "a", text: "" }, {}] } });
  assert.deepEqual(r.segments, []);
});

test("a new tiltStartTime resets everything", () => {
  const m = new ViewModel();
  m.merge({
    tiltStartTime: "t0",
    uiResources: [{ metadata: { name: "api" } }],
    logList: { spans: { "s1": { manifestName: "api" } } },
  });
  const r = m.merge({ tiltStartTime: "t1", uiResources: [{ metadata: { name: "web" } }] });
  assert.equal(r.restarted, true);
  assert.deepEqual(m.list().map((x) => x.metadata?.name), ["web"]);
  // The old span map is gone, so its segments no longer route to "api".
  const after = m.merge({ logList: { segments: [{ spanId: "s1", text: "stale\n" }] } });
  assert.equal(after.segments[0].resource, undefined);
});

test("the same tiltStartTime is not a restart", () => {
  const m = new ViewModel();
  m.merge({ tiltStartTime: "t0", uiResources: [{ metadata: { name: "api" } }] });
  const r = m.merge({ tiltStartTime: "t0", uiResources: [] });
  assert.equal(r.restarted, false);
  assert.equal(m.list().length, 1);
});

test("a resource with no name is ignored", () => {
  const m = new ViewModel();
  const r = m.merge({ uiResources: [{ status: { order: 1 } }] });
  assert.equal(r.resourcesChanged, false);
  assert.deepEqual(m.list(), []);
});

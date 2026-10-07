import assert from "node:assert/strict";
import path from "node:path";
import { test } from "node:test";
import {
  describeTiltfiles,
  downArgs,
  tiltfilePathFromEngineDump,
  upArgs,
} from "../src/tiltfile";

const ROOT = path.join(path.sep, "repo");
const p = (...parts: string[]) => path.join(ROOT, ...parts);

test("describeTiltfiles puts the workspace root Tiltfile first", () => {
  const entries = describeTiltfiles(
    [p("demo-k8s", "Tiltfile"), p("Tiltfile"), p("demo", "Tiltfile")],
    [ROOT],
  );
  // The root wins on depth. Sibling order is whatever localeCompare says.
  assert.equal(entries[0].label, "Tiltfile");
  assert.deepEqual(
    entries.slice(1).map((e) => e.label).sort(),
    [path.join("demo", "Tiltfile"), path.join("demo-k8s", "Tiltfile")].sort(),
  );
});

test("describeTiltfiles sorts shallowest first, then by name", () => {
  const entries = describeTiltfiles(
    [p("a", "b", "Tiltfile"), p("z", "Tiltfile"), p("a", "Tiltfile")],
    [ROOT],
  );
  assert.deepEqual(
    entries.map((e) => e.label),
    [
      path.join("a", "Tiltfile"),
      path.join("z", "Tiltfile"),
      path.join("a", "b", "Tiltfile"),
    ],
  );
});

test("describeTiltfiles reports the directory to run tilt in", () => {
  const [entry] = describeTiltfiles([p("demo", "Tiltfile")], [ROOT]);
  assert.equal(entry.dir, p("demo"));
  assert.equal(entry.file, p("demo", "Tiltfile"));
});

test("describeTiltfiles labels against the nearest workspace folder", () => {
  const nested = p("packages", "app");
  const [entry] = describeTiltfiles([path.join(nested, "Tiltfile")], [ROOT, nested]);
  assert.equal(entry.label, "Tiltfile", "the nested folder owns the label");
});

test("describeTiltfiles falls back to the full path outside any folder", () => {
  const outside = path.join(path.sep, "elsewhere", "Tiltfile");
  const [entry] = describeTiltfiles([outside], [ROOT]);
  assert.equal(entry.label, outside);
});

test("upArgs only passes a port when it is not the default", () => {
  assert.deepEqual(upArgs(10350), ["up"]);
  assert.deepEqual(upArgs(10351), ["up", "--port", "10351"]);
});

test("downArgs takes no port", () => {
  assert.deepEqual(downArgs(), ["down"]);
});

test("tiltfilePathFromEngineDump reads the running Tiltfile", () => {
  const dump = {
    TiltBuildInfo: { Version: "0.37.8" },
    DesiredTiltfilePath: "/repo/demo/Tiltfile",
  };
  assert.equal(tiltfilePathFromEngineDump(dump), "/repo/demo/Tiltfile");
});

test("tiltfilePathFromEngineDump rejects anything unusable", () => {
  for (const dump of [undefined, null, "", 42, {}, { DesiredTiltfilePath: "" }, { DesiredTiltfilePath: "  " }, { DesiredTiltfilePath: 7 }]) {
    assert.equal(tiltfilePathFromEngineDump(dump), undefined, JSON.stringify(dump));
  }
});

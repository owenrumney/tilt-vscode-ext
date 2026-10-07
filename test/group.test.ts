import assert from "node:assert/strict";
import { test } from "node:test";
import { TILTFILE, UNLABELED, groupResources, resourceLabels } from "../src/group";
import { UIResource } from "../src/types";

function res(name: string, labels?: Record<string, string>): UIResource {
  return { metadata: { name, labels } };
}

function shape(resources: UIResource[]): [string, string[]][] {
  return groupResources(resources).map((g) => [
    g.label,
    g.resources.map((r) => r.metadata?.name ?? ""),
  ]);
}

test("resourceLabels uses values and skips prefixed keys", () => {
  const r = res("api", {
    group: "cloud-data-1-app",
    "app.kubernetes.io/name": "api",
    empty: "",
  });
  assert.deepEqual(resourceLabels(r), ["cloud-data-1-app"]);
  assert.deepEqual(resourceLabels(res("api")), []);
});

test("groups sort A-Z, then unlabeled, then Tiltfile", () => {
  const resources = [
    res("(Tiltfile)"),
    res("loose"),
    res("etl", { g: "cloud-data-2-infra" }),
    res("api", { g: "cloud-data-1-app" }),
  ];
  assert.deepEqual(shape(resources), [
    ["cloud-data-1-app", ["api"]],
    ["cloud-data-2-infra", ["etl"]],
    [UNLABELED, ["loose"]],
    [TILTFILE, ["(Tiltfile)"]],
  ]);
});

test("a resource with two labels appears in both groups", () => {
  const resources = [res("api", { a: "app", b: "infra" })];
  assert.deepEqual(shape(resources), [
    ["app", ["api"]],
    ["infra", ["api"]],
  ]);
});

test("no labels means no groups, so the tree stays flat", () => {
  assert.deepEqual(groupResources([res("(Tiltfile)"), res("api")]), []);
  assert.deepEqual(groupResources([]), []);
});

test("resource order within a group is preserved", () => {
  const resources = [
    res("b", { g: "x" }),
    res("a", { g: "x" }),
  ];
  assert.deepEqual(shape(resources), [["x", ["b", "a"]]]);
});

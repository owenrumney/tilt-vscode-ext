import assert from "node:assert/strict";
import path from "node:path";
import { test } from "node:test";

// Plain CommonJS, so it is required rather than imported.
const script = require(
  path.resolve(__dirname, "../../scripts/package.js"),
) as {
  parseSemver(tag: string): { tag: string; parts: number[] } | null;
  compareSemverDesc(a: { parts: number[] }, b: { parts: number[] }): number;
  resolveReleaseTag(gitTags?: string): string | null;
  isTransient(output: string): boolean;
};

test("parseSemver accepts x.y.z with or without a v", () => {
  assert.deepEqual(script.parseSemver("v1.2.3")?.parts, [1, 2, 3]);
  assert.deepEqual(script.parseSemver("1.2.3")?.parts, [1, 2, 3]);
  for (const bad of ["v1.2", "v1.2.3-rc1", "main", "", "v1.2.3.4"]) {
    assert.equal(script.parseSemver(bad), null, bad);
  }
});

test("compareSemverDesc sorts newest first", () => {
  const tags = ["v1.2.3", "v1.10.0", "v1.2.10", "v2.0.0"]
    .map((t) => script.parseSemver(t)!)
    .sort(script.compareSemverDesc)
    .map((t) => t.tag);
  assert.deepEqual(tags, ["v2.0.0", "v1.10.0", "v1.2.10", "v1.2.3"]);
});

test("resolveReleaseTag prefers GITHUB_REF_NAME", () => {
  const prev = process.env.GITHUB_REF_NAME;
  process.env.GITHUB_REF_NAME = "v3.4.5";
  assert.equal(script.resolveReleaseTag(""), "v3.4.5");
  process.env.GITHUB_REF_NAME = "main";
  assert.equal(script.resolveReleaseTag("v1.0.0\nv1.1.0\n"), "v1.1.0");
  assert.equal(script.resolveReleaseTag("nightly\n"), null);
  assert.equal(script.resolveReleaseTag(""), null);
  if (prev === undefined) {
    delete process.env.GITHUB_REF_NAME;
  } else {
    process.env.GITHUB_REF_NAME = prev;
  }
});

test("isTransient only matches stalls worth retrying", () => {
  for (const out of ["Request timeout", "ETIMEDOUT", "socket hang up", "503"]) {
    assert.ok(script.isTransient(out), out);
  }
  for (const out of ["401 Unauthorized", "invalid token", ""]) {
    assert.equal(script.isTransient(out), false, out);
  }
});

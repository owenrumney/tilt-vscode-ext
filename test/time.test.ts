import assert from "node:assert/strict";
import { test } from "node:test";
import { formatAge, formatDuration, parseTime } from "../src/time";

test("parseTime treats Tilt's zero time as unset", () => {
  assert.equal(parseTime("0001-01-01T00:00:00Z"), undefined);
  assert.equal(parseTime(undefined), undefined);
  assert.equal(parseTime(""), undefined);
  assert.equal(parseTime("not a time"), undefined);
  assert.equal(parseTime("2026-10-07T17:22:53.464555Z"), Date.parse("2026-10-07T17:22:53.464Z"));
});

test("formatDuration picks the precision the length deserves", () => {
  const cases: [number, string][] = [
    [44, "44ms"],
    [999, "999ms"],
    [1000, "1.0s"],
    [4234, "4.2s"],
    [59_900, "59.9s"],
    [60_000, "1m"],
    [125_000, "2m 5s"],
  ];
  for (const [ms, want] of cases) {
    assert.equal(formatDuration(ms), want, `${ms}`);
  }
});

test("formatAge coarsens as it gets older", () => {
  const now = Date.parse("2026-10-07T18:00:00Z");
  const cases: [string, string][] = [
    ["2026-10-07T18:00:00Z", "just now"],
    ["2026-10-07T17:59:35Z", "25s ago"],
    ["2026-10-07T17:20:00Z", "40m ago"],
    ["2026-10-07T15:00:00Z", "3h ago"],
    ["2026-10-04T18:00:00Z", "3d ago"],
  ];
  for (const [iso, want] of cases) {
    assert.equal(formatAge(Date.parse(iso), now), want, iso);
  }
});

test("formatAge does not go negative on clock skew", () => {
  const now = Date.parse("2026-10-07T18:00:00Z");
  assert.equal(formatAge(Date.parse("2026-10-07T18:00:30Z"), now), "just now");
});

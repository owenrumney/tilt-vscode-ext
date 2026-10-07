import assert from "node:assert/strict";
import { test } from "node:test";
import {
  LogLevel,
  LogLine,
  LogStore,
  TILT_KEY,
  lineFilter,
  normalizeLevel,
} from "../src/logstore";

function texts(lines: { text: string }[]): string[] {
  return lines.map((l) => l.text);
}

function line(level: LogLevel, text: string): LogLine {
  return { key: "a", level, text, spans: [{ text }] };
}

test("segments are assembled into whole lines", () => {
  const store = new LogStore();
  const first = store.append([{ resource: "api", text: "hello wo" }]);
  assert.deepEqual(texts(first), []);
  const second = store.append([{ resource: "api", text: "rld\nnext\npart" }]);
  assert.deepEqual(texts(second), ["hello world", "next"]);
  // The unterminated tail still shows, so a prompt is not swallowed.
  assert.deepEqual(texts(store.lines("api")), ["hello world", "next", "part"]);
});

test("logs route by resource, Tilt's own under TILT_KEY", () => {
  const store = new LogStore();
  store.append([
    { resource: "api", text: "a\n" },
    { text: "tilt\n" },
    { resource: "web", text: "w\n" },
  ]);
  assert.deepEqual(texts(store.lines("api")), ["a"]);
  assert.deepEqual(texts(store.lines("web")), ["w"]);
  assert.deepEqual(texts(store.lines(TILT_KEY)), ["tilt"]);
  assert.deepEqual(store.lines("missing"), []);
});

test("counts report errors and warnings", () => {
  const store = new LogStore();
  store.append([
    { resource: "api", text: "boom\n", level: "ERROR" },
    { resource: "api", text: "careful\n", level: "WARN" },
    { resource: "api", text: "fine\n", level: "INFO" },
  ]);
  assert.deepEqual(store.counts("api"), {
    debug: 0,
    info: 1,
    warn: 1,
    error: 1,
  });
});

test("clear drops buffers and partial lines", () => {
  const store = new LogStore();
  store.append([{ resource: "api", text: "done\npartial" }]);
  store.clear();
  assert.deepEqual(store.lines("api"), []);
});

test("normalizeLevel folds Tilt's level names", () => {
  assert.equal(normalizeLevel("FATAL"), "error");
  assert.equal(normalizeLevel("warn"), "warn");
  assert.equal(normalizeLevel("VERBOSE"), "debug");
  assert.equal(normalizeLevel(undefined), "info");
  assert.equal(normalizeLevel("nonsense"), "info");
});

test("lineFilter keeps the level and above", () => {
  const keep = lineFilter({ minLevel: "warn" });
  assert.equal(keep(line("error", "x")), true);
  assert.equal(keep(line("warn", "x")), true);
  assert.equal(keep(line("info", "x")), false);
});

test("lineFilter matches text, case-insensitively", () => {
  const keep = lineFilter({ minLevel: "debug", text: "Boom" });
  assert.equal(keep(line("info", "kaboom!")), true);
  assert.equal(keep(line("info", "quiet")), false);
});

test("lineFilter treats /re/ as a regexp and ignores a broken one", () => {
  const keep = lineFilter({ minLevel: "debug", text: "/po(d|ds)-\\d+/" });
  assert.equal(keep(line("info", "pod-12 up")), true);
  assert.equal(keep(line("info", "pod up")), false);

  const broken = lineFilter({ minLevel: "debug", text: "/pod(/" });
  assert.equal(broken(line("info", "anything")), true);
});

test("escape codes are stripped from the text and kept as spans", () => {
  const store = new LogStore();
  store.append([{ resource: "api", text: "\u001b[34mSTEP 1\u001b[0m done\n" }]);
  const [first] = store.lines("api");
  assert.equal(first.text, "STEP 1 done");
  assert.deepEqual(first.spans, [
    { text: "STEP 1", color: "var(--vscode-terminal-ansiBlue)" },
    { text: " done" },
  ]);
});

test("a filter matches the stripped text, not the escape codes", () => {
  const store = new LogStore();
  store.append([{ resource: "api", text: "\u001b[31merror: boom\u001b[0m\n" }]);
  const keep = lineFilter({ minLevel: "debug", text: "error: boom" });
  assert.equal(store.lines("api").filter(keep).length, 1);
});

import assert from "node:assert/strict";
import { test } from "node:test";
import {
  isTiltProcess,
  listenerPidCommand,
  parseListenerPid,
  parseProcessName,
  processNameCommand,
} from "../src/process";

test("listenerPidCommand asks the right tool per platform", () => {
  assert.deepEqual(listenerPidCommand(10350, "darwin"), {
    command: "lsof",
    args: ["-nP", "-iTCP:10350", "-sTCP:LISTEN", "-t"],
  });
  assert.deepEqual(listenerPidCommand(10350, "linux").command, "lsof");
  assert.deepEqual(listenerPidCommand(10350, "win32").command, "netstat");
});

test("parseListenerPid reads lsof output", () => {
  // Real output from `lsof -nP -iTCP:10350 -sTCP:LISTEN -t`.
  assert.equal(parseListenerPid("63858\n", 10350, "darwin"), 63858);
  assert.equal(parseListenerPid("63858\n63859\n", 10350, "linux"), 63858);
  assert.equal(parseListenerPid("", 10350, "darwin"), undefined);
  assert.equal(parseListenerPid("\n  \n", 10350, "darwin"), undefined);
});

test("parseListenerPid matches the local port column on Windows", () => {
  const out = [
    "  TCP    0.0.0.0:135            0.0.0.0:0              LISTENING       900",
    "  TCP    127.0.0.1:10350        0.0.0.0:0              LISTENING       4242",
    "  TCP    127.0.0.1:52000        127.0.0.1:10350        ESTABLISHED     7777",
  ].join("\n");
  assert.equal(parseListenerPid(out, 10350, "win32"), 4242);
});

test("parseListenerPid does not match a port that merely ends the same", () => {
  const out = "  TCP    0.0.0.0:110350   0.0.0.0:0    LISTENING    999";
  assert.equal(parseListenerPid(out, 10350, "win32"), undefined);
});

test("parseListenerPid ignores a connection to the port", () => {
  const out = "  TCP    127.0.0.1:52000   127.0.0.1:10350   ESTABLISHED   7777";
  assert.equal(parseListenerPid(out, 10350, "win32"), undefined);
});

test("processNameCommand asks the right tool per platform", () => {
  assert.equal(processNameCommand(42, "darwin").command, "ps");
  assert.equal(processNameCommand(42, "win32").command, "tasklist");
});

test("parseProcessName strips a path and reads the CSV on Windows", () => {
  assert.equal(parseProcessName("tilt\n", "darwin"), "tilt");
  assert.equal(parseProcessName("/opt/homebrew/bin/tilt\n", "linux"), "tilt");
  assert.equal(parseProcessName('"tilt.exe","4242","Console","1","80 K"', "win32"), "tilt.exe");
  assert.equal(parseProcessName("", "darwin"), undefined);
});

test("isTiltProcess accepts only tilt", () => {
  for (const name of ["tilt", "TILT", "tilt.exe", "/usr/local/bin/tilt".split("/").pop()]) {
    assert.equal(isTiltProcess(name), true, String(name));
  }
  for (const name of [undefined, "", "node", "tiltfile-lsp", "tilty", "kubectl"]) {
    assert.equal(isTiltProcess(name), false, String(name));
  }
});

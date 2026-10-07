import assert from "node:assert/strict";
import { test } from "node:test";
import { isLoopbackHost } from "../src/token";

test("isLoopbackHost accepts the local machine only", () => {
  for (const host of [
    "localhost",
    "LOCALHOST",
    " localhost ",
    "127.0.0.1",
    "127.1.2.3",
    "::1",
    "[::1]",
    "0.0.0.0",
    "",
  ]) {
    assert.equal(isLoopbackHost(host), true, host);
  }
});

test("isLoopbackHost rejects anywhere the on-disk token must not go", () => {
  for (const host of [
    "attacker.example",
    "localhost.attacker.example",
    "127.0.0.1.attacker.example",
    "10.0.0.5",
    "192.168.1.9",
    "example.com",
    "0x7f000001",
  ]) {
    assert.equal(isLoopbackHost(host), false, host);
  }
});

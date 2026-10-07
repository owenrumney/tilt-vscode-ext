import assert from "node:assert/strict";
import { test } from "node:test";
import { parseAnsi, stripAnsi } from "../src/ansi";

const ESC = "\u001b";

test("plain text is one span", () => {
  assert.deepEqual(parseAnsi("hello"), [{ text: "hello" }]);
  assert.deepEqual(parseAnsi(""), []);
});

test("colour codes split the line into runs", () => {
  const spans = parseAnsi(`${ESC}[34mSTEP 1/1${ESC}[0m — Deploying`);
  assert.deepEqual(spans, [
    { text: "STEP 1/1", color: "var(--vscode-terminal-ansiBlue)" },
    { text: " — Deploying" },
  ]);
});

test("bright, bold and reset are tracked", () => {
  const spans = parseAnsi(`${ESC}[1;91mboom${ESC}[0mquiet`);
  assert.deepEqual(spans, [
    { text: "boom", bold: true, color: "var(--vscode-terminal-ansiBrightRed)" },
    { text: "quiet" },
  ]);
});

test("256-colour and truecolour become hex", () => {
  assert.deepEqual(parseAnsi(`${ESC}[38;5;196mred`), [
    { text: "red", color: "#ff0000" },
  ]);
  assert.deepEqual(parseAnsi(`${ESC}[38;2;18;52;86mrgb`), [
    { text: "rgb", color: "#123456" },
  ]);
  // Low indexes stay on the theme's palette.
  assert.deepEqual(parseAnsi(`${ESC}[38;5;4mblue`), [
    { text: "blue", color: "var(--vscode-terminal-ansiBlue)" },
  ]);
});

test("background colours are dropped without eating the text", () => {
  assert.deepEqual(parseAnsi(`${ESC}[48;5;196mon red`), [{ text: "on red" }]);
});

test("non-colour escapes and hyperlinks are removed", () => {
  const link = `${ESC}]8;;http://localhost${ESC}\\label`;
  assert.deepEqual(parseAnsi(link), [{ text: "label" }]);
  assert.deepEqual(parseAnsi(`${ESC}[2Kclear`), [{ text: "clear" }]);
});

test("private, intermediate and short escapes are removed", () => {
  assert.deepEqual(parseAnsi(`${ESC}[?25lworking${ESC}[?25h`), [
    { text: "working" },
  ]);
  assert.deepEqual(parseAnsi(`${ESC}[?1049hfoo`), [{ text: "foo" }]);
  assert.deepEqual(parseAnsi(`${ESC}[0 qtext`), [{ text: "text" }]);
  assert.deepEqual(parseAnsi(`${ESC}(Bplain`), [{ text: "plain" }]);
  assert.equal(stripAnsi(`${ESC}[?25lBuilding${ESC}[?25h`), "Building");
});

test("colour still applies around a private escape", () => {
  assert.deepEqual(parseAnsi(`${ESC}[?25l${ESC}[31mred${ESC}[0m`), [
    { text: "red", color: "var(--vscode-terminal-ansiRed)" },
  ]);
});

test("stripAnsi leaves the readable text", () => {
  assert.equal(stripAnsi(`${ESC}[34mApplying YAML${ESC}[0m`), "Applying YAML");
});

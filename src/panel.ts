import * as vscode from "vscode";
import {
  LEVELS,
  LogFilter,
  LogLevel,
  LogLine,
  LogStore,
  TILT_KEY,
  lineFilter,
} from "./logstore";

/** Lines the webview holds. The store keeps more; Copy still exports all of them. */
const WINDOW = 2000;

/** One reused webview showing the log of a single resource at a time. */
export class LogPanel implements vscode.Disposable {
  private panel?: vscode.WebviewPanel;
  private key = TILT_KEY;
  private filter: LogFilter = { minLevel: "debug", text: "" };

  constructor(private readonly store: LogStore) {}

  show(key: string): void {
    this.key = key;
    if (!this.panel) {
      this.panel = this.create();
    }
    this.panel.title = key === TILT_KEY ? "Tilt" : `Tilt: ${key}`;
    this.panel.reveal(this.panel.viewColumn ?? vscode.ViewColumn.Beside, true);
    this.render();
  }

  /** Streams the lines belonging to the shown resource. */
  append(lines: LogLine[]): void {
    if (!this.panel) {
      return;
    }
    const forKey = lines.filter((l) => l.key === this.key);
    if (forKey.length === 0) {
      return;
    }
    const mine = forKey.filter(lineFilter(this.filter));
    if (mine.length) {
      void this.panel.webview.postMessage({ type: "append", lines: mine });
    }
    this.postCounts();
  }

  clear(): void {
    this.render();
  }

  dispose(): void {
    this.panel?.dispose();
    this.panel = undefined;
  }

  private create(): vscode.WebviewPanel {
    const panel = vscode.window.createWebviewPanel(
      "tilt.logs",
      "Tilt",
      { viewColumn: vscode.ViewColumn.Beside, preserveFocus: true },
      {
        enableScripts: true,
        retainContextWhenHidden: true,
        enableFindWidget: true,
      },
    );
    panel.webview.html = html(panel.webview, WINDOW);
    panel.webview.onDidReceiveMessage((msg) => {
      if (msg?.type === "copy") {
        void this.copy();
      } else if (msg?.type === "filter") {
        this.filter = {
          minLevel: LEVELS.includes(msg.minLevel) ? msg.minLevel : "debug",
          text: typeof msg.text === "string" ? msg.text : "",
        };
        this.render();
      }
    });
    panel.onDidDispose(() => {
      this.panel = undefined;
    });
    return panel;
  }

  /** Copies the lines the filter leaves, without the escape codes. */
  private async copy(): Promise<void> {
    const lines = this.store.lines(this.key).filter(lineFilter(this.filter));
    await vscode.env.clipboard.writeText(lines.map((l) => l.text).join("\n"));
    vscode.window.setStatusBarMessage(
      `Tilt: copied ${lines.length} log lines`,
      3000,
    );
  }

  private render(): void {
    if (!this.panel) {
      return;
    }
    const keep = lineFilter(this.filter);
    void this.panel.webview.postMessage({
      type: "reset",
      resource: this.key === TILT_KEY ? "Tilt" : this.key,
      filter: this.filter,
      lines: this.store.lines(this.key).filter(keep).slice(-WINDOW),
      counts: this.store.counts(this.key),
    });
  }

  private postCounts(): void {
    void this.panel?.webview.postMessage({
      type: "counts",
      counts: this.store.counts(this.key),
    });
  }
}

function html(webview: vscode.Webview, window: number): string {
  const nonce = Math.random().toString(36).slice(2);
  const levelOptions = (["debug", "info", "warn", "error"] as LogLevel[])
    .map((l) => `<option value="${l}">${LEVEL_LABELS[l]}</option>`)
    .join("");

  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src ${webview.cspSource} 'unsafe-inline'; script-src 'nonce-${nonce}';">
<style>
  body { margin: 0; font-family: var(--vscode-editor-font-family); font-size: var(--vscode-editor-font-size); color: var(--vscode-foreground); }
  header { position: sticky; top: 0; display: flex; gap: 8px; align-items: center; padding: 6px 10px; background: var(--vscode-editor-background); border-bottom: 1px solid var(--vscode-panel-border); }
  header .name { font-weight: 600; margin-right: auto; }
  select, input[type="search"], button { font: inherit; color: var(--vscode-input-foreground); background: var(--vscode-input-background); border: 1px solid var(--vscode-input-border, transparent); padding: 2px 6px; }
  input[type="search"] { min-width: 220px; }
  button { cursor: pointer; }
  button:hover { background: var(--vscode-button-secondaryHoverBackground, var(--vscode-input-background)); }
  .follow { display: inline-flex; align-items: center; gap: 5px; white-space: nowrap; user-select: none; }
  .follow input { margin: 0; accent-color: var(--vscode-focusBorder); }
  #log { margin: 0; padding: 8px 10px; white-space: pre-wrap; word-break: break-word; font-family: var(--vscode-editor-font-family); }
  .line.error { color: var(--vscode-charts-red); }
  .line.warn { color: var(--vscode-charts-yellow); }
  .line.debug { color: var(--vscode-descriptionForeground); }
  .counts { color: var(--vscode-descriptionForeground); }
</style>
</head>
<body>
<header>
  <span class="name" id="name"></span>
  <span class="counts" id="counts"></span>
  <select id="level" title="Minimum level">${levelOptions}</select>
  <input id="text" type="search" placeholder="Filter text or /regexp/">
  <button id="copy" title="Copy the lines shown">Copy</button>
  <label class="follow"><input id="follow" type="checkbox" checked>Follow</label>
</header>
<pre id="log"></pre>
<script nonce="${nonce}">
  const vscode = acquireVsCodeApi();
  const WINDOW = ${window};
  const log = document.getElementById('log');
  const level = document.getElementById('level');
  const text = document.getElementById('text');
  const follow = document.getElementById('follow');
  const name = document.getElementById('name');
  const counts = document.getElementById('counts');

  function append(lines) {
    const frag = document.createDocumentFragment();
    for (const line of lines) {
      const el = document.createElement('div');
      el.className = 'line ' + line.level;
      for (const span of line.spans) {
        el.appendChild(styled(span));
      }
      frag.appendChild(el);
    }
    log.appendChild(frag);
    while (log.childElementCount > WINDOW) {
      log.removeChild(log.firstElementChild);
    }
    if (follow.checked) {
      window.scrollTo(0, document.body.scrollHeight);
    }
  }

  function styled(span) {
    const el = document.createElement('span');
    el.textContent = span.text;
    if (span.color) { el.style.color = span.color; }
    if (span.bold) { el.style.fontWeight = 'bold'; }
    if (span.italic) { el.style.fontStyle = 'italic'; }
    if (span.underline) { el.style.textDecoration = 'underline'; }
    if (span.dim) { el.style.opacity = '0.7'; }
    return el;
  }

  function sendFilter() {
    vscode.postMessage({ type: 'filter', minLevel: level.value, text: text.value });
  }
  level.addEventListener('change', sendFilter);
  text.addEventListener('input', debounce(sendFilter, 150));
  document.getElementById('copy').addEventListener('click', () => {
    vscode.postMessage({ type: 'copy' });
  });

  function debounce(fn, ms) {
    let t;
    return () => { clearTimeout(t); t = setTimeout(fn, ms); };
  }

  window.addEventListener('message', (event) => {
    const msg = event.data;
    if (msg.type === 'reset') {
      name.textContent = msg.resource;
      level.value = msg.filter.minLevel;
      if (text.value !== msg.filter.text) { text.value = msg.filter.text; }
      log.textContent = '';
      append(msg.lines);
      showCounts(msg.counts);
    } else if (msg.type === 'append') {
      append(msg.lines);
    } else if (msg.type === 'counts') {
      showCounts(msg.counts);
    }
  });

  function showCounts(c) {
    counts.textContent = c.error + ' errors · ' + c.warn + ' warnings';
  }
</script>
</body>
</html>`;
}

const LEVEL_LABELS: Record<LogLevel, string> = {
  debug: "All levels",
  info: "Info and above",
  warn: "Warnings and errors",
  error: "Errors only",
};

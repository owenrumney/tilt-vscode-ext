import * as vscode from "vscode";
import { TiltConfig, baseUrl, outboundToken } from "./config";
import {
  TiltfileEntry,
  describeTiltfiles,
  downArgs,
  tiltfilePathFromEngineDump,
  upArgs,
} from "./tiltfile";

const EXCLUDE = "**/{node_modules,.git,dist,out}/**";
// "tilt up" never exits, so it owns its terminal. Sending "tilt down" to the
// same one types into the running process stdin instead of a shell, which is
// why reusing a single terminal made the down button do nothing.
const UP_TERMINAL = "tilt up";
const DOWN_TERMINAL = "tilt down";

/** Every Tiltfile in the open folders, the root one first. */
export async function findTiltfiles(): Promise<TiltfileEntry[]> {
  const found = await vscode.workspace.findFiles(
    "**/Tiltfile",
    EXCLUDE,
    // More than a handful means the quick pick is the wrong UI anyway.
    64,
  );
  const roots = (vscode.workspace.workspaceFolders ?? []).map(
    (f) => f.uri.fsPath,
  );
  return describeTiltfiles(
    found.filter((u) => u.scheme === "file").map((u) => u.fsPath),
    roots,
  );
}

/**
 * Which Tiltfile to act on. A single one is used without asking; several
 * prompt, because guessing wrong starts the wrong stack.
 */
export async function pickTiltfile(
  entries: TiltfileEntry[],
  placeHolder: string,
): Promise<TiltfileEntry | undefined> {
  if (entries.length <= 1) {
    return entries[0];
  }
  const picked = await vscode.window.showQuickPick(
    entries.map((e) => ({ label: e.label, description: e.dir, entry: e })),
    { placeHolder },
  );
  return picked?.entry;
}

export async function tiltUp(config: TiltConfig): Promise<void> {
  const entries = await findTiltfiles();
  if (entries.length === 0) {
    vscode.window.showWarningMessage(
      "Tilt: no Tiltfile found in the open folders.",
    );
    return;
  }
  const entry = await pickTiltfile(entries, "Which Tiltfile to run?");
  if (!entry) {
    return;
  }
  if (await isUiListening(config)) {
    const open = "Open Tilt UI";
    const anyway = "Start anyway";
    const choice = await vscode.window.showWarningMessage(
      `Tilt is already serving on port ${config.port}.`,
      open,
      anyway,
    );
    if (choice === open) {
      await vscode.commands.executeCommand("tilt.openInBrowser");
      return;
    }
    if (choice !== anyway) {
      return;
    }
  }
  run(UP_TERMINAL, entry, upArgs(config.port));
}

export async function tiltDown(config: TiltConfig): Promise<void> {
  // Ask the running Tilt which Tiltfile it was started with, so the button
  // stops the session that is actually up — including one started outside the
  // editor. Only fall back to guessing when nothing answers.
  const running = await runningTiltfile(config);
  if (running) {
    run(DOWN_TERMINAL, entryForPath(running), downArgs());
    return;
  }

  const entries = await findTiltfiles();
  if (entries.length === 0) {
    vscode.window.showWarningMessage(
      "Tilt: no Tiltfile found in the open folders, and no running Tilt to ask.",
    );
    return;
  }
  const entry = await pickTiltfile(entries, "Which Tiltfile to stop?");
  if (!entry) {
    return;
  }
  run(DOWN_TERMINAL, entry, downArgs());
}

/** The absolute Tiltfile path a running Tilt reports, if one is reachable. */
async function runningTiltfile(config: TiltConfig): Promise<string | undefined> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 2000);
  try {
    const res = await fetch(`${baseUrl(config)}/api/dump/engine`, {
      headers: { "X-Tilt-Token": outboundToken(config) },
      signal: controller.signal,
    });
    if (!res.ok) {
      return undefined;
    }
    return tiltfilePathFromEngineDump(await res.json());
  } catch {
    // Not running, not reachable, or not answering in time.
    return undefined;
  } finally {
    clearTimeout(timer);
  }
}

function entryForPath(file: string): TiltfileEntry {
  const roots = (vscode.workspace.workspaceFolders ?? []).map(
    (f) => f.uri.fsPath,
  );
  return describeTiltfiles([file], roots)[0];
}

/**
 * Runs tilt in a terminal rather than a task: `tilt up` does not exit, prints
 * its own progress, and needs Ctrl-C to stop. A task would hide all three.
 */
function run(name: string, entry: TiltfileEntry, args: string[]): void {
  const existing = vscode.window.terminals.find((t) => t.name === name);
  const terminal =
    existing ?? vscode.window.createTerminal({ name, cwd: entry.dir });
  terminal.show();
  terminal.sendText(`cd ${quote(entry.dir)} && tilt ${args.join(" ")}`);
}

/** A HEAD on the UI port: a 200 or a 403 both mean something is serving. */
async function isUiListening(config: TiltConfig): Promise<boolean> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), 500);
  try {
    await fetch(`http://${config.host}:${config.port}/`, {
      method: "HEAD",
      signal: controller.signal,
    });
    return true;
  } catch {
    return false;
  } finally {
    clearTimeout(timer);
  }
}

function quote(dir: string): string {
  return /^[\w./\\:-]+$/.test(dir) ? dir : `'${dir.replace(/'/g, "'\\''")}'`;
}

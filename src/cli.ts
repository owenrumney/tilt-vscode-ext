import { execFile } from "child_process";
import * as vscode from "vscode";
import { TiltConfig, baseUrl, outboundToken } from "./config";
import {
  Platform,
  isTiltProcess,
  listenerPidCommand,
  parseListenerPid,
  parseProcessName,
  processNameCommand,
} from "./process";
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

// The Tiltfile this extension last started, so down knows what to tear down
// after the session has already been stopped.
const LAST_TILTFILE_KEY = "tilt.lastTiltfile";

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

export async function tiltUp(
  config: TiltConfig,
  context: vscode.ExtensionContext,
  log: vscode.LogOutputChannel,
): Promise<void> {
  log.info("tilt.up invoked");
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
      // The only route from a toolbar button to a browser, and only from up.
      log.info("tilt.up: already serving, opening the Tilt UI instead");
      await vscode.commands.executeCommand("tilt.openInBrowser");
      return;
    }
    if (choice !== anyway) {
      return;
    }
  }
  await context.workspaceState.update(LAST_TILTFILE_KEY, entry.file);
  run(UP_TERMINAL, entry, upArgs(config.port), log);
}

/**
 * Runs `tilt down` against the session that is up.
 *
 * Three sources for "which Tiltfile", in order of how much they are trusted.
 * The quick pick is the last resort, not the default: when a Tilt is running
 * it already knows the answer, and asking the user to repeat it is both
 * tedious and a chance to tear down the wrong stack.
 */
export async function tiltDown(
  config: TiltConfig,
  context: vscode.ExtensionContext,
  log: vscode.LogOutputChannel,
): Promise<void> {
  log.info("tilt.down invoked");

  const known = await knownTiltfile(config, context, log);
  if (known) {
    run(DOWN_TERMINAL, entryForPath(known), downArgs(), log);
    return;
  }

  const entries = await findTiltfiles();
  if (entries.length === 0) {
    vscode.window.showWarningMessage(
      "Tilt: no Tiltfile found in the open folders, and no running Tilt to ask.",
    );
    return;
  }
  const entry = await pickTiltfile(entries, "Which Tiltfile to tear down?");
  if (!entry) {
    return;
  }
  // Only confirm when the Tiltfile was guessed rather than known.
  if (!(await confirmDown(entry))) {
    return;
  }
  run(DOWN_TERMINAL, entry, downArgs(), log);
}

/** The running Tiltfile, else the last one this extension ran, else nothing. */
async function knownTiltfile(
  config: TiltConfig,
  context: vscode.ExtensionContext,
  log: vscode.LogOutputChannel,
): Promise<string | undefined> {
  const running = await runningTiltfile(config);
  if (running) {
    log.info(`tilt.down: the running Tilt reports ${running}`);
    return running;
  }
  const remembered = context.workspaceState.get<string>(LAST_TILTFILE_KEY);
  if (remembered) {
    log.info(`tilt.down: nothing is running; using the last one started, ${remembered}`);
    return remembered;
  }
  log.info("tilt.down: no running Tilt and nothing remembered, asking");
  return undefined;
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
/**
 * Runs a tilt command in a terminal.
 *
 * Always a new terminal, never a reused one. `sendText` does not reach a
 * shell when the terminal already runs a foreground process — it types into
 * that process's stdin. Reusing one terminal is how `cd … && tilt down` ended
 * up being typed at a running `tilt up`, which swallowed it silently.
 *
 * `cwd` is set on the terminal, so the `cd` is belt and braces: it makes the
 * directory visible in the scrollback, which matters when the answer to
 * "which Tiltfile?" came from somewhere the user cannot see.
 */
function run(
  name: string,
  entry: TiltfileEntry,
  args: string[],
  log: vscode.LogOutputChannel,
): void {
  const terminal = vscode.window.createTerminal({ name, cwd: entry.dir });
  terminal.show();
  const command = `cd ${quote(entry.dir)} && tilt ${args.join(" ")}`;
  log.info(`[${name}] ${command}`);
  terminal.sendText(command);
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

/**
 * Stops a running Tilt by signalling the process that holds the UI port.
 *
 * Signalling beats typing Ctrl-C into a terminal because it also stops a
 * session this extension did not start, which the welcome view explicitly
 * invites. Tilt has no API for stopping itself — its HTTP surface is view,
 * dump, trigger, snapshot and nothing else — so a signal is the only route.
 */
export async function tiltStop(
  config: TiltConfig,
  log: vscode.LogOutputChannel,
): Promise<void> {
  log.info("tilt.stop invoked");

  const pid = await tiltPid(config, log);
  if (pid === undefined) {
    await stopViaTerminal(log);
    return;
  }

  // SIGINT, which is what Ctrl-C sends, so Tilt runs its normal shutdown.
  log.info(`tilt.stop: sending SIGINT to pid ${pid}`);
  try {
    process.kill(pid, "SIGINT");
  } catch (err) {
    log.error(`tilt.stop: could not signal ${pid}: ${err}`);
    vscode.window.showErrorMessage(`Tilt: could not stop process ${pid}: ${err}`);
    return;
  }

  if (await waitForExit(pid, log)) {
    log.info(`tilt.stop: pid ${pid} exited`);
    vscode.window.setStatusBarMessage("Tilt stopped.", 3000);
    return;
  }
  log.warn(`tilt.stop: pid ${pid} still running after SIGINT`);
  const force = "Force Stop";
  const choice = await vscode.window.showWarningMessage(
    `Tilt (pid ${pid}) has not exited. Force it?`,
    force,
  );
  if (choice === force) {
    try {
      process.kill(pid, "SIGKILL");
      log.info(`tilt.stop: sent SIGKILL to pid ${pid}`);
    } catch (err) {
      log.error(`tilt.stop: could not kill ${pid}: ${err}`);
    }
  }
}

/**
 * The pid of the Tilt on the configured port, or undefined.
 *
 * Two checks, because `tilt.port` can name any port: the engine dump has to
 * answer, and the process has to be called tilt. Signalling whatever happens
 * to hold a port is not something to do on a user's machine.
 */
async function tiltPid(
  config: TiltConfig,
  log: vscode.LogOutputChannel,
): Promise<number | undefined> {
  if (!(await runningTiltfile(config))) {
    log.info(`tilt.stop: nothing answered on port ${config.port}`);
    return undefined;
  }
  const platform = process.platform as Platform;
  const lookup = listenerPidCommand(config.port, platform);
  const out = await capture(lookup.command, lookup.args);
  if (out === undefined) {
    log.warn(`tilt.stop: ${lookup.command} is not available`);
    return undefined;
  }
  const pid = parseListenerPid(out, config.port, platform);
  if (pid === undefined) {
    log.warn(`tilt.stop: nothing is listening on port ${config.port}`);
    return undefined;
  }

  const nameCmd = processNameCommand(pid, platform);
  const nameOut = await capture(nameCmd.command, nameCmd.args);
  const name = nameOut === undefined ? undefined : parseProcessName(nameOut, platform);
  if (!isTiltProcess(name)) {
    log.warn(`tilt.stop: pid ${pid} is ${name ?? "unknown"}, not tilt; refusing to signal`);
    return undefined;
  }
  return pid;
}

/** Ctrl-C into our own terminal, for when the pid could not be resolved. */
async function stopViaTerminal(log: vscode.LogOutputChannel): Promise<void> {
  // The most recent, since a new terminal is created per run.
  const terminal = [...vscode.window.terminals]
    .reverse()
    .find((t) => t.name === UP_TERMINAL);
  if (!terminal) {
    log.warn("tilt.stop: no tilt on the configured port and no terminal to interrupt");
    vscode.window.showWarningMessage(
      "Tilt: nothing to stop. No Tilt is answering on the configured port, " +
        "and this extension did not start one.",
    );
    return;
  }
  terminal.show();
  // ETX with no newline: a newline would run whatever the prompt holds once
  // tilt has exited.
  terminal.sendText("\u0003", false);
  log.info(`tilt.stop: sent Ctrl-C to the "${UP_TERMINAL}" terminal`);
}

/** True once the process is gone. Signal zero tests for existence. */
async function waitForExit(
  pid: number,
  log: vscode.LogOutputChannel,
): Promise<boolean> {
  for (let i = 0; i < 20; i++) {
    await new Promise((r) => setTimeout(r, 250));
    try {
      process.kill(pid, 0);
    } catch {
      return true;
    }
  }
  log.warn(`tilt.stop: pid ${pid} was still alive after 5s`);
  return false;
}

/** Command output, or undefined when the tool is missing or fails. */
function capture(command: string, args: string[]): Promise<string | undefined> {
  return new Promise((resolve) => {
    execFile(command, args, { timeout: 3000 }, (err, stdout) => {
      // lsof exits non-zero when nothing matches, which is not an error here.
      resolve(err && !stdout ? undefined : stdout);
    });
  });
}

/**
 * `tilt down` deletes what the Tiltfile deployed. That is a cluster change,
 * not a way to stop Tilt, so it asks first and says which Tiltfile.
 */
async function confirmDown(entry: TiltfileEntry): Promise<boolean> {
  const proceed = "Delete Resources";
  const choice = await vscode.window.showWarningMessage(
    `Delete the resources deployed by ${entry.label}?`,
    {
      modal: true,
      detail: "Runs tilt down. This does not stop a running tilt up.",
    },
    proceed,
  );
  return choice === proceed;
}

import * as vscode from "vscode";

/**
 * This extension's own previous identity. It contributes the same view
 * container, the same `tilt.resources` view and the same commands, so with
 * both installed every toolbar button and every welcome block appears twice.
 *
 * Renaming does not uninstall the old one, and VS Code can keep loading a
 * leftover extension directory after an uninstall has deregistered it.
 */
const PREDECESSOR_ID = "owenrumney.tilt-viewer";

/**
 * The official Tilt extension. It contributes the same `tiltfile` language id
 * and, until this extension moved to `source.tiltfile.tilt-tools`, the same
 * grammar scope. With both installed, which grammar highlights a Tiltfile is
 * not something either extension controls.
 *
 * It was last published in July 2024 and provides editing support only, which
 * this extension supersedes.
 */
const OFFICIAL_ID = "tilt-dev.Tiltfile";

const DISMISSED_KEY = "tilt.conflictNoticeDismissed";

/**
 * Warns about extensions that duplicate this one's contributions. The
 * predecessor is reported every time, because a doubled sidebar is broken
 * rather than merely untidy; the grammar overlap is dismissible.
 */
export async function warnOnConflicts(
  context: vscode.ExtensionContext,
  log: vscode.LogOutputChannel,
): Promise<void> {
  await warnOnPredecessor(log);
  await warnOnGrammarOverlap(context, log);
}

/** Two copies of the sidebar. Not dismissible: nothing here still works. */
async function warnOnPredecessor(log: vscode.LogOutputChannel): Promise<void> {
  if (!vscode.extensions.getExtension(PREDECESSOR_ID)) {
    return;
  }
  log.error(
    `${PREDECESSOR_ID} is still installed. It was renamed to this extension, ` +
      `and both contribute the same view, so the sidebar is duplicated.`,
  );
  const uninstall = "Uninstall It";
  const choice = await vscode.window.showErrorMessage(
    "Tilt Viewer is still installed. It was renamed to Tilt Tools, and having " +
      "both duplicates every button in the Tilt sidebar.",
    uninstall,
  );
  if (choice === uninstall) {
    await vscode.commands.executeCommand(
      "workbench.extensions.uninstallExtension",
      PREDECESSOR_ID,
    );
    await promptReload();
  }
}

/** Both grammars claim the tiltfile language. Dismissible. */
async function warnOnGrammarOverlap(
  context: vscode.ExtensionContext,
  log: vscode.LogOutputChannel,
): Promise<void> {
  if (!vscode.extensions.getExtension(OFFICIAL_ID)) {
    return;
  }
  log.warn(
    `${OFFICIAL_ID} is installed. It contributes the same tiltfile language, ` +
      `so syntax highlighting may come from either extension.`,
  );
  if (context.globalState.get<boolean>(DISMISSED_KEY)) {
    return;
  }

  const manage = "Show Extension";
  const dismiss = "Don't Show Again";
  const choice = await vscode.window.showWarningMessage(
    "Tilt Tools and the tilt-dev Tiltfile extension both provide Tiltfile " +
      "highlighting. Disable the other one to avoid a conflict.",
    manage,
    dismiss,
  );
  if (choice === manage) {
    await vscode.commands.executeCommand("workbench.extensions.search", OFFICIAL_ID);
  }
  if (choice === dismiss) {
    await context.globalState.update(DISMISSED_KEY, true);
  }
}

async function promptReload(): Promise<void> {
  const reload = "Reload Window";
  const choice = await vscode.window.showInformationMessage(
    "Tilt Viewer removed. Reload to clear the duplicated sidebar.",
    reload,
  );
  if (choice === reload) {
    await vscode.commands.executeCommand("workbench.action.reloadWindow");
  }
}

import * as vscode from "vscode";

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
 * Warns once if the official Tiltfile extension is also installed. Dismissal
 * is remembered, because a notice that returns every window is worse than the
 * conflict it reports.
 */
export async function warnOnGrammarConflict(
  context: vscode.ExtensionContext,
  log: vscode.LogOutputChannel,
): Promise<void> {
  const other = vscode.extensions.getExtension(OFFICIAL_ID);
  if (!other) {
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

import * as fs from "fs";
import * as os from "os";
import * as path from "path";
import * as vscode from "vscode";
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
} from "vscode-languageclient/node";

const BINARY = os.platform() === "win32" ? "tiltfile-lsp.exe" : "tiltfile-lsp";

/**
 * Starts the Tiltfile language server, or returns undefined when it is turned
 * off or its binary is missing. A missing binary is not an error: the tree view
 * is the extension's main job and must keep working without it.
 */
export function startLanguageServer(
  context: vscode.ExtensionContext,
  log: vscode.LogOutputChannel,
): LanguageClient | undefined {
  const config = vscode.workspace.getConfiguration("tilt");
  if (!config.get<boolean>("lsp.enabled", true)) {
    log.info("Tiltfile language server disabled by tilt.lsp.enabled.");
    return undefined;
  }

  const command = serverPath(context, configuredPath(config));
  if (!command) {
    log.warn(
      `Tiltfile language server not found. Set tilt.lsp.path, or build it with "make lsp".`,
    );
    return undefined;
  }

  const serverOptions: ServerOptions = {
    run: { command },
    debug: { command },
  };
  const clientOptions: LanguageClientOptions = {
    documentSelector: [{ scheme: "file", language: "tiltfile" }],
    outputChannel: log,
  };

  const client = new LanguageClient(
    "tiltfile-lsp",
    "Tiltfile Language Server",
    serverOptions,
    clientOptions,
  );
  client.start().catch((err: unknown) => {
    log.error(`Tiltfile language server failed to start: ${err}`);
  });
  log.info(`Tiltfile language server started from ${command}.`);
  return client;
}

/**
 * The path from user settings only. A workspace value would let a cloned
 * repository's .vscode/settings.json choose the binary this extension spawns.
 */
function configuredPath(config: vscode.WorkspaceConfiguration): string | undefined {
  return config.inspect<string>("lsp.path")?.globalValue;
}

/** The configured path if it exists, else the bundled binary, else nothing. */
function serverPath(
  context: vscode.ExtensionContext,
  configured?: string,
): string | undefined {
  const candidates = [
    configured,
    path.join(context.extensionPath, "bin", BINARY),
  ];
  return candidates.find((p): p is string => !!p && isExecutable(p));
}

function isExecutable(file: string): boolean {
  try {
    fs.accessSync(file, fs.constants.X_OK);
    return fs.statSync(file).isFile();
  } catch {
    return false;
  }
}

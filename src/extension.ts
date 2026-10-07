import * as vscode from "vscode";
import { TiltClient } from "./client";
import { CONNECTION_KEYS, baseUrl, readConfig } from "./config";
import { LogManager } from "./logs";
import { ViewModel } from "./model";
import { ResourceItem, ResourceTree } from "./tree";

const REFRESH_DEBOUNCE_MS = 100;

export function activate(context: vscode.ExtensionContext): void {
  const model = new ViewModel();
  const logs = new LogManager();
  const tree = new ResourceTree(model);
  const client = new TiltClient(readConfig());

  const status = vscode.window.createStatusBarItem(
    vscode.StatusBarAlignment.Left,
    0,
  );
  status.command = "tilt.reconnect";
  status.show();

  let refreshTimer: NodeJS.Timeout | undefined;
  const scheduleRefresh = () => {
    if (refreshTimer) {
      return;
    }
    refreshTimer = setTimeout(() => {
      refreshTimer = undefined;
      tree.refresh();
    }, REFRESH_DEBOUNCE_MS);
  };

  const renderStatus = () => {
    switch (client.connectionState) {
      case "connected": {
        const count = model.list().length;
        status.text = `$(zap) Tilt: ${count}`;
        status.tooltip = "Connected to Tilt. Click to reconnect.";
        break;
      }
      case "connecting":
        status.text = "$(sync~spin) Tilt: connecting";
        status.tooltip = "Connecting to Tilt.";
        break;
      default:
        status.text = "$(plug) Tilt: disconnected";
        status.tooltip = "Tilt is not reachable. Click to reconnect.";
    }
  };
  renderStatus();

  client.onView((view) => {
    const result = model.merge(view);
    if (result.restarted) {
      logs.clearAll();
    }
    logs.append(result.segments);
    if (result.resourcesChanged) {
      scheduleRefresh();
      renderStatus();
    }
    if (view.fatalError) {
      vscode.window.showErrorMessage(`Tilt: ${view.fatalError}`);
    }
  });

  client.onStateChange((state) => {
    if (state === "disconnected") {
      // A reconnect replays the log from the start, so drop what we have.
      model.clear();
      logs.clearAll();
      tree.refresh();
    }
    renderStatus();
  });

  const reconnect = () => {
    model.clear();
    logs.clearAll();
    tree.refresh();
    client.restart(readConfig());
  };

  context.subscriptions.push(
    client,
    logs,
    status,
    vscode.window.createTreeView("tilt.resources", { treeDataProvider: tree }),
    vscode.commands.registerCommand("tilt.connect", () => client.start()),
    vscode.commands.registerCommand("tilt.disconnect", () => client.stop()),
    vscode.commands.registerCommand("tilt.reconnect", reconnect),
    vscode.commands.registerCommand("tilt.showTiltLogs", () => logs.showTilt()),
    vscode.commands.registerCommand("tilt.showLogs", async (item?: ResourceItem) => {
      const name = item?.resource.metadata?.name ?? (await pickResource(model));
      if (name) {
        logs.show(name);
      }
    }),
    vscode.commands.registerCommand("tilt.trigger", async (item?: ResourceItem) => {
      const name = item?.resource.metadata?.name ?? (await pickResource(model));
      if (!name) {
        return;
      }
      try {
        await client.trigger(name);
        logs.show(name);
      } catch (err) {
        vscode.window.showErrorMessage(`Tilt: could not trigger ${name}: ${err}`);
      }
    }),
    vscode.commands.registerCommand("tilt.openInBrowser", () => {
      const url = `${baseUrl(readConfig())}/`;
      void vscode.env.openExternal(vscode.Uri.parse(url));
    }),
    vscode.workspace.onDidChangeConfiguration((e) => {
      if (CONNECTION_KEYS.some((k) => e.affectsConfiguration(k))) {
        reconnect();
      }
    }),
  );

  if (readConfig().autoConnect) {
    client.start();
  }
}

export function deactivate(): void {
  // Disposables registered on the context handle teardown.
}

async function pickResource(model: ViewModel): Promise<string | undefined> {
  const names = model
    .list()
    .map((r) => r.metadata?.name)
    .filter((n): n is string => !!n);
  if (names.length === 0) {
    vscode.window.showInformationMessage("Tilt: no resources. Is Tilt running?");
    return undefined;
  }
  return vscode.window.showQuickPick(names, { placeHolder: "Tilt resource" });
}

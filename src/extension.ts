import * as vscode from "vscode";
import { TiltClient } from "./client";
import { CONNECTION_KEYS, TiltConfig, baseUrl, readConfig } from "./config";
import { LogStore, TILT_KEY } from "./logstore";
import { ViewModel } from "./model";
import { LogPanel } from "./panel";
import {
  ALL_STATUSES,
  STATUS_LABELS,
  isWebUrl,
  resourceLinks,
  statusIcon,
} from "./status";
import { ResourceItem, ResourceTree, TreeNode } from "./tree";

const REFRESH_DEBOUNCE_MS = 100;

export function activate(context: vscode.ExtensionContext): void {
  const model = new ViewModel();
  const store = new LogStore();
  const panel = new LogPanel(store);
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
  void vscode.commands.executeCommand(
    "setContext",
    "tilt.state",
    client.connectionState,
  );

  client.onView((view) => {
    const result = model.merge(view);
    if (result.restarted) {
      store.clear();
      panel.clear();
    }
    panel.append(store.append(result.segments));
    if (result.resourcesChanged) {
      scheduleRefresh();
      renderStatus();
    }
    if (view.fatalError) {
      vscode.window.showErrorMessage(`Tilt: ${view.fatalError}`);
    }
  });

  client.onStateChange((state) => {
    if (state !== "connected") {
      // A reconnect replays the log from the start, so drop what we have.
      model.clear();
      store.clear();
      panel.clear();
    }
    tree.setConnected(state === "connected");
    tree.refresh();
    renderStatus();
    void vscode.commands.executeCommand("setContext", "tilt.state", state);
  });

  const reconnect = () => {
    model.clear();
    store.clear();
    panel.clear();
    tree.refresh();
    client.restart(readConfig());
  };

  const view = vscode.window.createTreeView("tilt.resources", {
    treeDataProvider: tree,
  });

  const applyStatusFilter = async () => {
    const picked = await vscode.window.showQuickPick(
      ALL_STATUSES.map((s) => ({
        label: `$(${statusIcon(s).replace("~spin", "")}) ${STATUS_LABELS[s]}`,
        status: s,
        picked: tree.statusFilter.includes(s),
      })),
      { canPickMany: true, title: "Show resources with status" },
    );
    if (!picked) {
      return;
    }
    tree.setStatusFilter(picked.map((p) => p.status));
    view.description = tree.isFiltered
      ? tree.statusFilter.map((s) => STATUS_LABELS[s]).join(", ")
      : undefined;
    void vscode.commands.executeCommand(
      "setContext",
      "tilt.filtered",
      tree.isFiltered,
    );
  };

  context.subscriptions.push(
    client,
    panel,
    status,
    view,
    vscode.commands.registerCommand("tilt.filterStatus", applyStatusFilter),
    vscode.commands.registerCommand("tilt.clearStatusFilter", () => {
      tree.setStatusFilter([]);
      view.description = undefined;
      void vscode.commands.executeCommand("setContext", "tilt.filtered", false);
    }),
    vscode.commands.registerCommand("tilt.connect", () => client.start()),
    vscode.commands.registerCommand("tilt.disconnect", () => client.stop()),
    vscode.commands.registerCommand("tilt.reconnect", reconnect),
    vscode.commands.registerCommand("tilt.showTiltLogs", () =>
      panel.show(TILT_KEY),
    ),
    vscode.commands.registerCommand("tilt.expandAll", () =>
      tree.setGroupsExpanded(true),
    ),
    vscode.commands.registerCommand("tilt.collapseAll", () =>
      tree.setGroupsExpanded(false),
    ),
    vscode.commands.registerCommand("tilt.showLogs", async (item?: TreeNode) => {
      const name = resourceName(item) ?? (await pickResource(model));
      if (name) {
        panel.show(name);
      }
    }),
    vscode.commands.registerCommand("tilt.trigger", async (item?: TreeNode) => {
      const name = resourceName(item) ?? (await pickResource(model));
      if (!name) {
        return;
      }
      try {
        await client.trigger(name);
        panel.show(name);
      } catch (err) {
        vscode.window.showErrorMessage(`Tilt: could not trigger ${name}: ${err}`);
      }
    }),
    vscode.commands.registerCommand("tilt.openInBrowser", () =>
      openUrl(readConfig(), `${baseUrl(readConfig())}/`),
    ),
    vscode.commands.registerCommand("tilt.openEndpoint", async (item?: TreeNode) => {
      const links =
        item instanceof ResourceItem ? resourceLinks(item.resource) : [];
      if (links.length === 0) {
        vscode.window.showInformationMessage("Tilt: this resource has no links.");
        return;
      }
      const url =
        links.length === 1
          ? links[0].url
          : (
              await vscode.window.showQuickPick(
                links.map((l) => ({ label: l.name, description: l.url })),
                { title: "Open endpoint" },
              )
            )?.description;
      if (url) {
        await openUrl(readConfig(), url);
      }
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

async function openUrl(config: TiltConfig, url: string): Promise<void> {
  if (!isWebUrl(url)) {
    vscode.window.showWarningMessage(`Tilt: refusing to open ${url}`);
    return;
  }
  const uri = vscode.Uri.parse(url);
  if (config.openInEditor) {
    try {
      await vscode.commands.executeCommand("simpleBrowser.show", uri.toString());
      return;
    } catch {
      // Simple Browser is missing in some builds; the real browser still works.
    }
  }
  void vscode.env.openExternal(uri);
}

function resourceName(item?: TreeNode): string | undefined {
  return item instanceof ResourceItem ? item.resource.metadata?.name : undefined;
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

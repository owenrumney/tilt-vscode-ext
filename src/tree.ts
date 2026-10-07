import * as vscode from "vscode";
import { ViewModel } from "./model";
import { resourceStatus, statusIcon, statusLabel } from "./status";
import { UIResource } from "./types";

export class ResourceItem extends vscode.TreeItem {
  constructor(readonly resource: UIResource) {
    const name = resource.metadata?.name ?? "(unnamed)";
    super(name, vscode.TreeItemCollapsibleState.None);

    const status = resourceStatus(resource);
    this.id = name;
    this.contextValue = "tiltResource";
    this.iconPath = new vscode.ThemeIcon(statusIcon(status));
    this.description = statusLabel(resource);
    this.tooltip = tooltip(resource);
    this.command = {
      command: "tilt.showLogs",
      title: "Show Logs",
      arguments: [this],
    };
  }
}

export class ResourceTree implements vscode.TreeDataProvider<ResourceItem> {
  private readonly changed = new vscode.EventEmitter<void>();
  readonly onDidChangeTreeData = this.changed.event;

  constructor(private readonly model: ViewModel) {}

  refresh(): void {
    this.changed.fire();
  }

  getTreeItem(element: ResourceItem): vscode.TreeItem {
    return element;
  }

  getChildren(element?: ResourceItem): ResourceItem[] {
    if (element) {
      return [];
    }
    return this.model.list().map((r) => new ResourceItem(r));
  }
}

function tooltip(r: UIResource): vscode.MarkdownString {
  const s = r.status ?? {};
  const lines = [
    `**${r.metadata?.name ?? "(unnamed)"}**`,
    "",
    `- update: \`${s.updateStatus ?? "none"}\``,
    `- runtime: \`${s.runtimeStatus ?? "none"}\``,
  ];
  if (s.k8sResourceInfo?.podName) {
    lines.push(`- pod: \`${s.k8sResourceInfo.podName}\``);
  }
  if (s.k8sResourceInfo?.podRestarts) {
    lines.push(`- restarts: ${s.k8sResourceInfo.podRestarts}`);
  }
  for (const link of s.endpointLinks ?? []) {
    if (link.url) {
      lines.push(`- [${link.name || link.url}](${link.url})`);
    }
  }
  const md = new vscode.MarkdownString(lines.join("\n"));
  md.isTrusted = false;
  return md;
}

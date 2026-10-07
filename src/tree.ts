import * as vscode from "vscode";
import { ResourceGroup, groupResources } from "./group";
import { ViewModel } from "./model";
import {
  ALL_STATUSES,
  ResourceStatus,
  buildSummary,
  conditionDetail,
  filterByStatus,
  hasLiveUpdate,
  podMessage,
  podObjects,
  resourceLinks,
  resourceStatus,
  resourceType,
  statusColor,
  statusIcon,
  statusLabel,
  waitingLabel,
  worstStatus,
} from "./status";
import { UIResource } from "./types";

export type TreeNode = GroupItem | ResourceItem;

export class GroupItem extends vscode.TreeItem {
  constructor(
    readonly group: ResourceGroup,
    state: vscode.TreeItemCollapsibleState,
    revision: number,
  ) {
    super(group.label, state);

    const healthy = group.resources.filter((r) =>
      ["ok", "running"].includes(resourceStatus(r)),
    ).length;
    // VS Code remembers expansion by id, so collapse/expand all needs a new one.
    this.id = `group:${revision}:${group.label}`;
    this.contextValue = "tiltGroup";
    this.iconPath = icon(worstStatus(group.resources));
    this.description = `${healthy}/${group.resources.length}`;
  }
}

export class ResourceItem extends vscode.TreeItem {
  constructor(readonly resource: UIResource, group?: string) {
    const name = resource.metadata?.name ?? "(unnamed)";
    super(name, vscode.TreeItemCollapsibleState.None);

    this.id = group ? `${group}/${name}` : name;
    // The link button hangs off this suffix.
    this.contextValue = resourceLinks(resource).length
      ? "tiltResourceLinks"
      : "tiltResource";
    this.iconPath = icon(resourceStatus(resource));
    this.description = statusLabel(resource);
    this.tooltip = tooltip(resource);
    this.command = {
      command: "tilt.showLogs",
      title: "Show Logs",
      arguments: [this],
    };
  }
}

export class ResourceTree implements vscode.TreeDataProvider<TreeNode> {
  private readonly changed = new vscode.EventEmitter<void>();
  readonly onDidChangeTreeData = this.changed.event;
  private groupState = vscode.TreeItemCollapsibleState.Expanded;
  private revision = 0;
  private allowed = new Set<ResourceStatus>(ALL_STATUSES);
  private connected = false;

  constructor(private readonly model: ViewModel) {}

  refresh(): void {
    this.changed.fire();
  }

  /** Resources are only shown while connected, so nothing stale survives. */
  setConnected(connected: boolean): void {
    if (this.connected !== connected) {
      this.connected = connected;
      this.refresh();
    }
  }

  /** The statuses the tree shows. An empty list means all of them. */
  setStatusFilter(statuses: ResourceStatus[]): void {
    this.allowed = new Set(statuses.length ? statuses : ALL_STATUSES);
    this.refresh();
  }

  get statusFilter(): ResourceStatus[] {
    return ALL_STATUSES.filter((s) => this.allowed.has(s));
  }

  get isFiltered(): boolean {
    return this.allowed.size !== ALL_STATUSES.length;
  }

  setGroupsExpanded(expanded: boolean): void {
    this.groupState = expanded
      ? vscode.TreeItemCollapsibleState.Expanded
      : vscode.TreeItemCollapsibleState.Collapsed;
    this.revision += 1;
    this.refresh();
  }

  getTreeItem(element: TreeNode): vscode.TreeItem {
    return element;
  }

  getChildren(element?: TreeNode): TreeNode[] {
    if (element instanceof GroupItem) {
      return this.connected
        ? element.group.resources.map(
            (r) => new ResourceItem(r, element.group.label),
          )
        : [];
    }
    if (element || !this.connected) {
      return [];
    }
    const resources = filterByStatus(this.model.list(), this.allowed);
    const groups = groupResources(resources);
    if (groups.length === 0) {
      return resources.map((r) => new ResourceItem(r));
    }
    return groups.map((g) => new GroupItem(g, this.groupState, this.revision));
  }
}

function icon(status: ResourceStatus): vscode.ThemeIcon {
  const color = statusColor(status);
  return new vscode.ThemeIcon(
    statusIcon(status),
    color ? new vscode.ThemeColor(color) : undefined,
  );
}

function tooltip(r: UIResource): vscode.MarkdownString {
  const s = r.status ?? {};
  const type = resourceType(r);
  const lines = [
    `**${r.metadata?.name ?? "(unnamed)"}**`,
    "",
    `- update: \`${s.updateStatus ?? "none"}\``,
    `- runtime: \`${s.runtimeStatus ?? "none"}\``,
  ];
  if (type) {
    lines.push(
      `- type: \`${type}\`${hasLiveUpdate(r) ? " (live update)" : ""}`,
    );
  }
  const build = buildSummary(r);
  if (build) {
    lines.push(`- last build: ${build}`);
  }
  const waiting = waitingLabel(r);
  if (waiting) {
    lines.push(`- ${waiting}`);
  }
  if (s.k8sResourceInfo?.podName) {
    lines.push(`- pod: \`${s.k8sResourceInfo.podName}\``);
  }
  if (s.k8sResourceInfo?.podRestarts) {
    lines.push(`- restarts: ${s.k8sResourceInfo.podRestarts}`);
  }
  const message = podMessage(r);
  if (message) {
    lines.push(`- ${truncate(message, 120)}`);
  }
  const notReady = conditionDetail(r, "Ready");
  if (notReady) {
    lines.push(`- not ready: ${truncate(notReady, 120)}`);
  }
  const objects = podObjects(r);
  if (objects.length) {
    lines.push(`- objects: ${objects.join(", ")}`);
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

// Pod messages carry the full pod id, which overflows a tooltip line.
function truncate(text: string, max: number): string {
  return text.length <= max ? text : `${text.slice(0, max - 1)}…`;
}

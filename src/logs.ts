import * as vscode from "vscode";
import { RoutedSegment } from "./model";

const TILT_CHANNEL = "Tilt";

/** One Output channel per resource, created the first time a log line arrives. */
export class LogManager implements vscode.Disposable {
  private channels = new Map<string, vscode.OutputChannel>();

  append(segments: RoutedSegment[]): void {
    for (const s of segments) {
      this.channel(s.resource ?? TILT_CHANNEL).append(s.text);
    }
  }

  show(resource: string): void {
    this.channel(resource).show(true);
  }

  showTilt(): void {
    this.channel(TILT_CHANNEL).show(true);
  }

  clearAll(): void {
    for (const channel of this.channels.values()) {
      channel.clear();
    }
  }

  dispose(): void {
    for (const channel of this.channels.values()) {
      channel.dispose();
    }
    this.channels.clear();
  }

  private channel(name: string): vscode.OutputChannel {
    let channel = this.channels.get(name);
    if (!channel) {
      const title = name === TILT_CHANNEL ? TILT_CHANNEL : `Tilt: ${name}`;
      channel = vscode.window.createOutputChannel(title);
      this.channels.set(name, channel);
    }
    return channel;
  }
}

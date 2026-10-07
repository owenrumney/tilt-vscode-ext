import * as vscode from "vscode";
import { findToken, isLoopbackHost } from "./token";

export interface TiltConfig {
  host: string;
  port: number;
  token: string;
  autoConnect: boolean;
  openInEditor: boolean;
  /** True when the token came from disk rather than from settings. */
  tokenFromFile: boolean;
}

export const CONNECTION_KEYS = ["tilt.host", "tilt.port", "tilt.token"];

export function readConfig(): TiltConfig {
  const c = vscode.workspace.getConfiguration("tilt");
  const configured = c.get<string>("token", "").trim();
  return {
    host: c.get<string>("host", "localhost"),
    port: c.get<number>("port", 10350),
    token: configured || findToken(),
    autoConnect: c.get<boolean>("autoConnect", true),
    openInEditor: c.get<boolean>("openInEditor", true),
    tokenFromFile: !configured,
  };
}

/**
 * The token to send, which is empty when the on-disk token would be leaving
 * this machine. See isLoopbackHost.
 */
export function outboundToken(c: TiltConfig): string {
  if (c.tokenFromFile && !isLoopbackHost(c.host)) {
    return "";
  }
  return c.token;
}

export function baseUrl(c: TiltConfig): string {
  return `http://${c.host}:${c.port}`;
}

export function wsUrl(c: TiltConfig, csrf: string): string {
  return `ws://${c.host}:${c.port}/ws/view?csrf=${encodeURIComponent(csrf)}`;
}

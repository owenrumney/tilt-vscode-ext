import * as vscode from "vscode";
import { findToken } from "./token";

export interface TiltConfig {
  host: string;
  port: number;
  token: string;
  autoConnect: boolean;
}

export const CONNECTION_KEYS = ["tilt.host", "tilt.port", "tilt.token"];

export function readConfig(): TiltConfig {
  const c = vscode.workspace.getConfiguration("tilt");
  return {
    host: c.get<string>("host", "localhost"),
    port: c.get<number>("port", 10350),
    token: c.get<string>("token", "").trim() || findToken(),
    autoConnect: c.get<boolean>("autoConnect", true),
  };
}

export function baseUrl(c: TiltConfig): string {
  return `http://${c.host}:${c.port}`;
}

export function wsUrl(c: TiltConfig, csrf: string): string {
  return `ws://${c.host}:${c.port}/ws/view?csrf=${encodeURIComponent(csrf)}`;
}

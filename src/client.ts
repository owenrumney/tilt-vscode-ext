import * as vscode from "vscode";
import WebSocket from "ws";
import { TiltConfig, baseUrl, outboundToken, wsUrl } from "./config";
import { View } from "./types";

const TOKEN_HEADER = "X-Tilt-Token";
/** model.BuildReasonFlagTriggerWeb (1 << 4). */
const BUILD_REASON_TRIGGER_WEB = 16;
const BACKOFF_MS = [1000, 2000, 4000, 8000, 16000, 30000];
const FETCH_TIMEOUT_MS = 5000;
/** Tilt can die without closing the socket, so liveness is checked here. */
const HEARTBEAT_MS = 15000;
const MISSED_PONGS = 2;

export type ConnectionState = "disconnected" | "connecting" | "connected";

export class TiltClient implements vscode.Disposable {
  private socket?: WebSocket;
  private retry?: NodeJS.Timeout;
  private heartbeat?: NodeJS.Timeout;
  private alive = false;
  private missed = 0;
  private attempt = 0;
  private stopped = true;
  private state: ConnectionState = "disconnected";

  private readonly onViewEmitter = new vscode.EventEmitter<View>();
  private readonly onStateEmitter = new vscode.EventEmitter<ConnectionState>();

  readonly onView = this.onViewEmitter.event;
  readonly onStateChange = this.onStateEmitter.event;

  constructor(private config: TiltConfig) {}

  get connectionState(): ConnectionState {
    return this.state;
  }

  start(): void {
    this.stopped = false;
    this.attempt = 0;
    this.open();
  }

  stop(): void {
    this.stopped = true;
    this.clearRetry();
    this.closeSocket();
    this.setState("disconnected");
  }

  restart(config: TiltConfig): void {
    this.config = config;
    this.stop();
    this.start();
  }

  async trigger(name: string): Promise<void> {
    const body = JSON.stringify({
      manifest_names: [name],
      build_reason: BUILD_REASON_TRIGGER_WEB,
    });
    const res = await this.request("/api/trigger", { method: "POST", body });
    // Tilt answers 200 with an error string in the body for a disabled resource.
    const text = (await res.text()).trim();
    if (!res.ok) {
      throw new Error(text || `${res.status} ${res.statusText}`);
    }
    if (text) {
      throw new Error(text);
    }
  }

  dispose(): void {
    this.stop();
    this.onViewEmitter.dispose();
    this.onStateEmitter.dispose();
  }

  private async open(): Promise<void> {
    if (this.stopped) {
      return;
    }
    this.setState("connecting");

    let csrf: string;
    try {
      csrf = await this.websocketToken();
    } catch (err) {
      this.scheduleRetry(err);
      return;
    }
    if (this.stopped) {
      return;
    }

    const socket = new WebSocket(wsUrl(this.config, csrf), {
      headers: { [TOKEN_HEADER]: outboundToken(this.config) },
    });
    this.socket = socket;

    socket.on("open", () => {
      this.attempt = 0;
      this.setState("connected");
      this.startHeartbeat(socket);
    });
    socket.on("message", (data) => {
      this.alive = true;
      this.handleMessage(data.toString());
    });
    socket.on("pong", () => {
      this.alive = true;
    });
    socket.on("error", () => {
      // "close" always follows; retry is scheduled there.
    });
    socket.on("close", () => {
      if (this.socket === socket) {
        this.socket = undefined;
        this.stopHeartbeat();
        this.scheduleRetry();
      }
    });
  }

  private handleMessage(raw: string): void {
    try {
      this.onViewEmitter.fire(JSON.parse(raw) as View);
    } catch (err) {
      console.error("tilt: unparseable view message", err);
    }
  }

  private async websocketToken(): Promise<string> {
    const res = await this.request("/api/websocket_token");
    if (!res.ok) {
      throw new Error(`websocket_token: ${res.status} ${res.statusText}`);
    }
    return (await res.text()).trim();
  }

  private async request(path: string, init: RequestInit = {}): Promise<Response> {
    const signal = AbortSignal.timeout(FETCH_TIMEOUT_MS);
    return fetch(`${baseUrl(this.config)}${path}`, {
      ...init,
      signal,
      headers: {
        ...(init.headers ?? {}),
        [TOKEN_HEADER]: outboundToken(this.config),
        "Content-Type": "application/json",
      },
    });
  }

  private scheduleRetry(err?: unknown): void {
    if (this.stopped || this.retry) {
      return;
    }
    this.setState("disconnected");
    if (err) {
      console.log(`tilt: connect failed, retrying: ${err}`);
    }
    const delay = BACKOFF_MS[Math.min(this.attempt, BACKOFF_MS.length - 1)];
    this.attempt += 1;
    this.retry = setTimeout(() => {
      this.retry = undefined;
      void this.open();
    }, delay);
  }

  private startHeartbeat(socket: WebSocket): void {
    this.stopHeartbeat();
    this.alive = true;
    this.missed = 0;
    this.heartbeat = setInterval(() => {
      if (this.alive) {
        this.missed = 0;
      } else if (++this.missed >= MISSED_PONGS) {
        // A long synchronous parse can delay one pong, so only a run of
        // missed ticks means the connection is dead but open.
        socket.terminate();
        return;
      }
      this.alive = false;
      socket.ping();
    }, HEARTBEAT_MS);
  }

  private stopHeartbeat(): void {
    if (this.heartbeat) {
      clearInterval(this.heartbeat);
      this.heartbeat = undefined;
    }
  }

  private clearRetry(): void {
    if (this.retry) {
      clearTimeout(this.retry);
      this.retry = undefined;
    }
  }

  private closeSocket(): void {
    const socket = this.socket;
    this.socket = undefined;
    this.stopHeartbeat();
    socket?.removeAllListeners();
    socket?.close();
  }

  private setState(state: ConnectionState): void {
    if (this.state !== state) {
      this.state = state;
      this.onStateEmitter.fire(state);
    }
  }
}

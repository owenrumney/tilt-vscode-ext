/**
 * Finding the process listening on a TCP port, so a running Tilt can be
 * stopped by signal rather than by typing Ctrl-C into a terminal.
 *
 * Signalling the PID works for a Tilt this extension did not start, which is
 * the common case: the welcome view invites `tilt up` in your own terminal.
 *
 * Node has no portable port-to-PID lookup, so this shells out. Both halves
 * are pure functions over the command output, which is what makes them
 * testable without a listening socket.
 */

export type Platform = "darwin" | "linux" | "win32";

/** The command that lists the PID listening on a port. */
export function listenerPidCommand(
  port: number,
  platform: Platform,
): { command: string; args: string[] } {
  if (platform === "win32") {
    return {
      command: "netstat",
      args: ["-ano", "-p", "tcp"],
    };
  }
  return {
    command: "lsof",
    args: ["-nP", `-iTCP:${port}`, "-sTCP:LISTEN", "-t"],
  };
}

/**
 * The PID from that command's output, or undefined when nothing listens.
 *
 * lsof -t prints one PID per line. netstat prints a table, so the port has to
 * be matched against the local address column rather than anywhere in a line:
 * ":10350" would also match a remote address or a different port's suffix.
 */
export function parseListenerPid(
  stdout: string,
  port: number,
  platform: Platform,
): number | undefined {
  if (platform !== "win32") {
    const first = stdout
      .split("\n")
      .map((l) => l.trim())
      .find((l) => /^\d+$/.test(l));
    return first ? Number(first) : undefined;
  }

  for (const line of stdout.split("\n")) {
    const cols = line.trim().split(/\s+/);
    // Proto, Local Address, Foreign Address, State, PID
    if (cols.length < 5 || cols[3] !== "LISTENING") {
      continue;
    }
    const localPort = cols[1].slice(cols[1].lastIndexOf(":") + 1);
    if (localPort !== String(port)) {
      continue;
    }
    const pid = Number(cols[4]);
    if (Number.isInteger(pid) && pid > 0) {
      return pid;
    }
  }
  return undefined;
}

/** The command that reports a process's executable name. */
export function processNameCommand(
  pid: number,
  platform: Platform,
): { command: string; args: string[] } {
  if (platform === "win32") {
    return { command: "tasklist", args: ["/FI", `PID eq ${pid}`, "/NH", "/FO", "CSV"] };
  }
  return { command: "ps", args: ["-p", String(pid), "-o", "comm="] };
}

/**
 * The executable name from that output. Checked before signalling, because
 * `tilt.port` can name any port and the listener may not be Tilt at all.
 */
export function parseProcessName(
  stdout: string,
  platform: Platform,
): string | undefined {
  const text = stdout.trim();
  if (!text) {
    return undefined;
  }
  if (platform === "win32") {
    // "tilt.exe","1234","Console","1","12,345 K"
    const first = text.split("\n")[0].split(",")[0].replace(/"/g, "").trim();
    return first || undefined;
  }
  // ps prints the full path when the binary was invoked by path.
  const line = text.split("\n")[0].trim();
  return line.slice(line.lastIndexOf("/") + 1) || undefined;
}

/** Whether a process name is Tilt, allowing for the Windows suffix. */
export function isTiltProcess(name: string | undefined): boolean {
  if (!name) {
    return false;
  }
  const base = name.toLowerCase().replace(/\.exe$/, "");
  return base === "tilt";
}

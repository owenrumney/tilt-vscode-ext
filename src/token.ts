import * as fs from "fs";
import * as os from "os";
import * as path from "path";

// Tilt writes its session token here on first run. ~/.windmill is the legacy name.
const TOKEN_FILES = [
  path.join(".tilt-dev", "token"),
  path.join(".windmill", "token"),
];

export function findToken(): string {
  const home = os.homedir();
  for (const rel of TOKEN_FILES) {
    try {
      const token = fs.readFileSync(path.join(home, rel), "utf8").trim();
      if (token) {
        return token;
      }
    } catch {
      // Missing file is the normal case for whichever dir Tilt did not use.
    }
  }
  return "";
}

/**
 * Hosts the on-disk token may be sent to.
 *
 * `tilt.host` is workspace-settable, so a cloned repository can point the
 * extension anywhere. The token in ~/.tilt-dev/token is an ambient credential
 * the user never typed for that host, so it travels to the local machine
 * only. A token set explicitly in settings is the user's choice and is sent
 * wherever they aimed it.
 */
export function isLoopbackHost(host: string): boolean {
  const h = host.trim().toLowerCase().replace(/^\[|\]$/g, "");
  return (
    h === "localhost" ||
    h === "127.0.0.1" ||
    h === "::1" ||
    h === "0.0.0.0" ||
    h === "" ||
    /^127\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(h)
  );
}

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

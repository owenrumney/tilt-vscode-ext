const { execFileSync, execSync } = require("child_process");
const fs = require("fs");
const path = require("path");

const EXT_DIR = path.resolve(__dirname, "..");
const SEMVER = /^(\d+)\.(\d+)\.(\d+)$/;

function parseSemver(tag) {
  const m = SEMVER.exec(String(tag).replace(/^v/, ""));
  if (!m) return null;
  return { tag, parts: [Number(m[1]), Number(m[2]), Number(m[3])] };
}

function compareSemverDesc(a, b) {
  for (let i = 0; i < 3; i++) {
    if (a.parts[i] !== b.parts[i]) return b.parts[i] - a.parts[i];
  }
  return 0;
}

function resolveReleaseTag(gitTags) {
  const refTag = process.env.GITHUB_REF_NAME;
  if (/^v/.test(refTag || "") && parseSemver(refTag)) {
    return refTag;
  }
  try {
    const out =
      gitTags ??
      execSync("git tag --points-at HEAD", { cwd: EXT_DIR, encoding: "utf-8" });
    const tags = out
      .split("\n")
      .map((s) => s.trim())
      .filter((s) => s.startsWith("v"))
      .map(parseSemver)
      .filter(Boolean)
      .sort(compareSemverDesc);
    if (tags.length > 0) {
      return tags[0].tag;
    }
  } catch {
    // Fall through to package.json version.
  }
  return null;
}

// A timed-out publish may never have landed, so it must be retried;
// --skip-duplicate makes that safe if the request did get through after all.
function isTransient(output) {
  return /request timeout|etimedout|econnreset|socket hang up|getaddrinfo|50\d\b/i.test(
    output,
  );
}

if (require.main !== module) {
  module.exports = { parseSemver, compareSemverDesc, resolveReleaseTag, isTransient };
  return;
}

const pkgPath = path.join(EXT_DIR, "package.json");
const pkg = JSON.parse(fs.readFileSync(pkgPath, "utf-8"));
const EXTENSION_ID = `${pkg.publisher}.${pkg.name}`;
const mode = process.argv[2] || "all";
if (!["all", "build", "publish"].includes(mode)) {
  console.error(`Usage: node scripts/package.js [build|publish|all]`);
  process.exit(2);
}

const tag = resolveReleaseTag();
const version = tag ? tag.replace(/^v/, "") : pkg.version;
// vsce rejects anything else, and the version is interpolated into a filename.
if (!SEMVER.test(version)) {
  console.error(`Refusing to build: "${version}" is not a x.y.z version.`);
  process.exit(2);
}
const vsix = path.join(EXT_DIR, `${pkg.name}-${version}.vsix`);

// Sync version from the release tag (v1.2.3 → 1.2.3) so package.json never drifts.
if (mode !== "publish") {
  if (!tag) {
    console.log("No release tag found, using version from package.json.");
  } else if (pkg.version !== version) {
    console.log(`Updating package.json version: ${pkg.version} → ${version}`);
    pkg.version = version;
    fs.writeFileSync(pkgPath, JSON.stringify(pkg, null, 2) + "\n");
  }

  console.log(`\nPackaging ${path.basename(vsix)}...`);
  execFileSync("npx", ["vsce", "package", "--no-dependencies", "--out", vsix], {
    cwd: EXT_DIR,
    stdio: "inherit",
  });
}

if (mode === "build") {
  console.log(`\nDone: ${path.basename(vsix)}`);
  return;
}

if (!fs.existsSync(vsix)) {
  console.error(`\n${path.basename(vsix)} is missing; run the build step first.`);
  process.exit(1);
}

// The marketplace stalls for minutes at a time; re-running later succeeds
// unchanged, so the wait between rounds is what recovers it.
const PUBLISH_TIMEOUT_MS = 45_000;
const ROUND_DELAYS_MS = [60_000, 180_000];
const MARKETPLACE = "VS Code Marketplace";
const failures = [];

function sleep(ms) {
  Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms);
}

function elapsed(since) {
  return `${((Date.now() - since) / 1000).toFixed(1)}s`;
}

// Returns "ok", "retry" (stalled — worth another round) or "failed".
function publishOnce(registry, argv, env) {
  const started = Date.now();
  try {
    process.stdout.write(
      execFileSync("npx", argv, {
        cwd: EXT_DIR,
        stdio: ["ignore", "pipe", "pipe"],
        encoding: "utf-8",
        timeout: PUBLISH_TIMEOUT_MS,
        env: { ...process.env, ...env },
      }) || "",
    );
    console.log(`  ${version} → ${registry} (${elapsed(started)})`);
    return "ok";
  } catch (err) {
    const output = `${err.stdout || ""}${err.stderr || ""}${err.message || ""}`;
    process.stdout.write(output);
    console.error(`  FAILED after ${elapsed(started)}: ${version} → ${registry}`);
    return isTransient(output) ? "retry" : "failed";
  }
}

function publish(registry, tokenEnv, build) {
  if (!process.env[tokenEnv]) {
    console.log(`\nSkipping ${registry} publish (no ${tokenEnv}).`);
    return;
  }
  const { argv, env } = build();
  for (let round = 0; ; round++) {
    console.log(
      `\nPublishing to ${registry}...` + (round ? ` (round ${round + 1})` : ""),
    );
    const result = publishOnce(registry, argv, env);
    if (result === "ok") {
      return;
    }
    if (result === "failed" || round >= ROUND_DELAYS_MS.length) {
      failures.push(`${version} → ${registry}`);
      return;
    }
    const delay = ROUND_DELAYS_MS[round];
    console.error(`  ${registry} stalled, retrying in ${delay / 1000}s`);
    sleep(delay);
  }
}

publish(MARKETPLACE, "VSCODE_PUBLISH_TOKEN", () => ({
  argv: ["vsce", "publish", "--skip-duplicate", "--packagePath", vsix],
  env: { VSCE_PAT: process.env.VSCODE_PUBLISH_TOKEN },
}));

publish("Open VSX", "OPVSX_PUBLISH_TOKEN", () => ({
  argv: ["ovsx", "publish", "--skip-duplicate", "--packagePath", vsix],
  env: { OVSX_PAT: process.env.OPVSX_PUBLISH_TOKEN },
}));

// Marketplace validation can take many minutes, so the poll is given room
// rather than failing the release on indexing lag.
const VERIFY_POLL_MS = [0, 15_000, 30_000, ...Array(14).fill(60_000)];

async function marketplaceVersions() {
  const res = await fetch(
    "https://marketplace.visualstudio.com/_apis/public/gallery/extensionquery",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Accept: "application/json;api-version=7.2-preview.1",
      },
      body: JSON.stringify({
        filters: [{ criteria: [{ filterType: 7, value: EXTENSION_ID }] }],
        flags: 51,
      }),
    },
  );
  if (!res.ok) {
    throw new Error(`gallery returned ${res.status} ${res.statusText}`);
  }
  const body = await res.json();
  const versions = body.results?.[0]?.extensions?.[0]?.versions ?? [];
  return new Set(versions.map((v) => v.version));
}

function dropFailure(registry) {
  const i = failures.indexOf(`${version} → ${registry}`);
  if (i >= 0) {
    failures.splice(i, 1);
  }
}

async function verifyMarketplace() {
  if (!process.env.VSCODE_PUBLISH_TOKEN) {
    return;
  }
  console.log(`\nVerifying ${EXTENSION_ID} ${version} on the Marketplace...`);
  for (const delay of VERIFY_POLL_MS) {
    if (delay) sleep(delay);
    try {
      if ((await marketplaceVersions()).has(version)) {
        console.log(`  ${version} is live`);
        // A publish that timed out but landed is a success, not a failure.
        dropFailure(MARKETPLACE);
        return;
      }
    } catch (err) {
      console.error(`  gallery query failed: ${err.message}`);
      continue;
    }
    console.log(`  waiting on ${version}`);
  }
  // The publish itself reported how it went; a slow gallery is not a failure.
  console.error(`  WARNING: ${version} not visible on the Marketplace yet.`);
}

(async () => {
  await verifyMarketplace();

  if (failures.length > 0) {
    console.error(`\n${failures.length} publish(es) failed:`);
    for (const f of failures) {
      console.error(`  ${f}`);
    }
    process.exit(1);
  }
  console.log(`\nDone: ${path.basename(vsix)}`);
})();

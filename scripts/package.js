const { execFileSync, execSync } = require("child_process");
const fs = require("fs");
const path = require("path");

const EXT_DIR = path.resolve(__dirname, "..");
const BIN_DIR = path.join(EXT_DIR, "bin");

// One .vsix per platform, each carrying one cross-compiled language server.
// Shipping all five binaries in one artifact would quintuple every download.
const TARGETS = [
  { vsce: "darwin-arm64", goos: "darwin", goarch: "arm64" },
  { vsce: "darwin-x64", goos: "darwin", goarch: "amd64" },
  { vsce: "linux-arm64", goos: "linux", goarch: "arm64" },
  { vsce: "linux-x64", goos: "linux", goarch: "amd64" },
  { vsce: "win32-x64", goos: "windows", goarch: "amd64" },
];

function binaryName(goos) {
  return goos === "windows" ? "tiltfile-lsp.exe" : "tiltfile-lsp";
}

// vsce preserves the file mode, and without the executable bit the extension
// ships a language server it cannot start.
function buildServer(target, serverVersion) {
  fs.rmSync(BIN_DIR, { recursive: true, force: true });
  fs.mkdirSync(BIN_DIR, { recursive: true });
  const out = path.join(BIN_DIR, binaryName(target.goos));
  // Stamped like the goreleaser build, so a bug report from the bundled
  // server names a version rather than "dev".
  execFileSync(
    "go",
    [
      "build",
      "-trimpath",
      `-ldflags=-s -w -X main.version=${serverVersion}`,
      "-o",
      out,
      "./cmd/tiltfile-lsp",
    ],
    {
      cwd: path.join(EXT_DIR, "lsp"),
      stdio: "inherit",
      env: { ...process.env, GOOS: target.goos, GOARCH: target.goarch, CGO_ENABLED: "0" },
    },
  );
  fs.chmodSync(out, 0o755);
  return out;
}
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
  module.exports = {
    parseSemver,
    compareSemverDesc,
    resolveReleaseTag,
    isTransient,
    TARGETS,
    binaryName,
  };
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
const vsixFor = (target) =>
  path.join(EXT_DIR, `${pkg.name}-${version}-${target.vsce}.vsix`);

// VSCE_TARGET builds one platform only, which is what a local install wants.
const wanted = process.env.VSCE_TARGET;
const targets = wanted ? TARGETS.filter((t) => t.vsce === wanted) : TARGETS;
if (targets.length === 0) {
  console.error(`Unknown VSCE_TARGET "${wanted}". One of: ${TARGETS.map((t) => t.vsce).join(", ")}`);
  process.exit(2);
}

// Sync version from the release tag (v1.2.3 → 1.2.3) so package.json never drifts.
if (mode !== "publish") {
  if (!tag) {
    console.log("No release tag found, using version from package.json.");
  } else if (pkg.version !== version) {
    console.log(`Updating package.json version: ${pkg.version} → ${version}`);
    pkg.version = version;
    fs.writeFileSync(pkgPath, JSON.stringify(pkg, null, 2) + "\n");
  }

  for (const target of targets) {
    const out = vsixFor(target);
    console.log(`\nPackaging ${path.basename(out)} (${target.goos}/${target.goarch})...`);
    buildServer(target, version);
    execFileSync(
      "npx",
      ["vsce", "package", "--no-dependencies", "--target", target.vsce, "--out", out],
      { cwd: EXT_DIR, stdio: "inherit" },
    );
  }
  fs.rmSync(BIN_DIR, { recursive: true, force: true });
}

if (mode === "build") {
  for (const target of targets) {
    const out = vsixFor(target);
    const mb = (fs.statSync(out).size / 1024 / 1024).toFixed(1);
    console.log(`  ${path.basename(out)} (${mb} MB)`);
  }
  return;
}

const missing = targets.map(vsixFor).filter((p) => !fs.existsSync(p));
if (missing.length > 0) {
  console.error(
    `\nMissing, so run the build step first:\n  ${missing.map((p) => path.basename(p)).join("\n  ")}`,
  );
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
function publishOnce(registry, label, argv, env) {
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
    console.log(`  ${label} → ${registry} (${elapsed(started)})`);
    return "ok";
  } catch (err) {
    const output = `${err.stdout || ""}${err.stderr || ""}${err.message || ""}`;
    process.stdout.write(output);
    console.error(`  FAILED after ${elapsed(started)}: ${label} → ${registry}`);
    return isTransient(output) ? "retry" : "failed";
  }
}

function publishFile(registry, label, argv, env) {
  for (let round = 0; ; round++) {
    if (round) {
      console.log(`  retrying ${label} (round ${round + 1})`);
    }
    const result = publishOnce(registry, label, argv, env);
    if (result === "ok") {
      return;
    }
    if (result === "failed" || round >= ROUND_DELAYS_MS.length) {
      failures.push(`${label} → ${registry}`);
      return;
    }
    const delay = ROUND_DELAYS_MS[round];
    console.error(`  ${registry} stalled, retrying in ${delay / 1000}s`);
    sleep(delay);
  }
}

// One call per .vsix. A partial publish is recoverable because both
// registries take --skip-duplicate, so a re-run is safe.
function publish(registry, tokenEnv, build) {
  if (!process.env[tokenEnv]) {
    console.log(`\nSkipping ${registry} publish (no ${tokenEnv}).`);
    return;
  }
  console.log(`\nPublishing to ${registry}...`);
  for (const target of targets) {
    const file = vsixFor(target);
    const { argv, env } = build(file);
    publishFile(registry, `${version} ${target.vsce}`, argv, env);
  }
}

publish(MARKETPLACE, "VSCODE_PUBLISH_TOKEN", (file) => ({
  argv: ["vsce", "publish", "--skip-duplicate", "--packagePath", file],
  env: { VSCE_PAT: process.env.VSCODE_PUBLISH_TOKEN },
}));

publish("Open VSX", "OPVSX_PUBLISH_TOKEN", (file) => ({
  argv: ["ovsx", "publish", "--skip-duplicate", "--packagePath", file],
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

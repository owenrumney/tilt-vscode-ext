// Downloads real Tiltfiles from public repositories, so the corpus tests in
// lsp/internal/corpus can be run by anyone.
//
//   node scripts/fetch-corpus.js [--out DIR] [--limit N] [--refresh]
//
// Needs the gh CLI, authenticated: GitHub code search requires a token.
// Files are cached, so a second run downloads nothing. The manifest records
// what was fetched and when, which is what makes two runs comparable.

const { execFileSync } = require("child_process");
const fs = require("fs");
const path = require("path");

const ROOT = path.resolve(__dirname, "..");
const DEFAULT_OUT = path.join(ROOT, ".corpus");

// Three queries rather than one: code search ranks and truncates, so each
// returns a different slice of the Tiltfiles in the wild.
const QUERIES = [
  "filename:Tiltfile local_resource",
  "filename:Tiltfile k8s_yaml",
  "filename:Tiltfile docker_build",
  "filename:Tiltfile k8s_resource",
];

function parseArgs(argv) {
  const args = { out: DEFAULT_OUT, limit: 60, refresh: false };
  for (let i = 0; i < argv.length; i++) {
    switch (argv[i]) {
      case "--out":
        args.out = path.resolve(argv[++i]);
        break;
      case "--limit":
        args.limit = Number(argv[++i]);
        break;
      case "--refresh":
        args.refresh = true;
        break;
      default:
        console.error(`Unknown argument: ${argv[i]}`);
        process.exit(2);
    }
  }
  if (!Number.isInteger(args.limit) || args.limit < 1 || args.limit > 100) {
    console.error("--limit must be between 1 and 100 (the search page size).");
    process.exit(2);
  }
  return args;
}

function gh(argv) {
  return execFileSync("gh", argv, {
    encoding: "utf-8",
    stdio: ["ignore", "pipe", "pipe"],
    maxBuffer: 64 * 1024 * 1024,
  });
}

function requireGh() {
  try {
    gh(["auth", "status"]);
  } catch {
    console.error(
      "gh is not available or not authenticated. Run: gh auth login",
    );
    process.exit(1);
  }
}

/** Every hit across the queries, deduplicated on repo plus path. */
function search(limit) {
  const found = new Map();
  for (const q of QUERIES) {
    let out;
    try {
      out = gh([
        "api",
        "-X",
        "GET",
        "search/code",
        "--raw-field",
        `q=${q}`,
        "--raw-field",
        `per_page=${limit}`,
        "--jq",
        '.items[] | "\\(.repository.full_name)\\t\\(.path)\\t\\(.sha)"',
      ]);
    } catch (err) {
      console.error(`  query failed: ${q}`);
      continue;
    }
    let added = 0;
    for (const line of out.trim().split("\n").filter(Boolean)) {
      const [repo, file, sha] = line.split("\t");
      const key = `${repo}:${file}`;
      if (!found.has(key)) {
        found.set(key, { repo, path: file, sha });
        added++;
      }
    }
    console.log(`  ${q} -> ${added} new`);
  }
  return [...found.values()];
}

/** A flat, collision-free filename, since the corpus is one directory. */
function localName(hit) {
  return `${hit.repo}_${hit.path}`.replace(/[^A-Za-z0-9._-]/g, "_");
}

function download(hit, dir) {
  const b64 = gh(["api", `repos/${hit.repo}/contents/${hit.path}`, "--jq", ".content"]);
  const text = Buffer.from(b64.replace(/\s/g, ""), "base64").toString("utf-8");
  if (!text.trim()) {
    throw new Error("empty file");
  }
  fs.writeFileSync(path.join(dir, localName(hit)), text);
  return text.length;
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  requireGh();
  fs.mkdirSync(args.out, { recursive: true });

  console.log(`Searching GitHub (per_page=${args.limit})...`);
  const hits = search(args.limit);
  if (hits.length === 0) {
    console.error("No Tiltfiles found. Is the search API rate limited?");
    process.exit(1);
  }

  let fetched = 0,
    cached = 0,
    failed = 0;
  for (const hit of hits) {
    const dest = path.join(args.out, localName(hit));
    if (!args.refresh && fs.existsSync(dest) && fs.statSync(dest).size > 0) {
      cached++;
      continue;
    }
    try {
      download(hit, args.out);
      fetched++;
    } catch {
      failed++;
    }
  }

  const manifest = {
    fetchedAt: new Date().toISOString(),
    queries: QUERIES,
    count: hits.length - failed,
    files: hits
      .map((h) => ({ repo: h.repo, path: h.path, sha: h.sha, local: localName(h) }))
      .sort((a, b) => a.local.localeCompare(b.local)),
  };
  fs.writeFileSync(
    path.join(args.out, "manifest.json"),
    JSON.stringify(manifest, null, 2) + "\n",
  );

  console.log(
    `\n${manifest.count} Tiltfiles in ${args.out} ` +
      `(${fetched} downloaded, ${cached} cached, ${failed} failed)`,
  );
  console.log(`Run the corpus tests:\n  TILT_CORPUS=${args.out} make lsp-corpus`);
}

main();

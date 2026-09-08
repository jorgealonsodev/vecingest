#!/usr/bin/env node
// Regenerates every file under packages/shared/src from
// api/openapi/openapi.yaml. This script is the only hand-written entry
// point in packages/shared's generation chain (tasks 7.1/7.2): it invokes
// openapi-typescript and openapi-zod-client and writes their output
// verbatim, plus a tiny deterministic openapi-fetch factory and barrel.
// Nothing this script writes under src/client/ or src/schemas/ may be
// hand-edited afterward (api-contract-generation: Generation Chain —
// "no type in packages/shared MAY be hand-written"). src/errors.ts is the
// one exception and is never touched by this script.
//
// Invoked by `make gen` via `pnpm --filter @vecingest/shared build`.

import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";
import fs from "node:fs";

const here = path.dirname(fileURLToPath(import.meta.url));
const pkgRoot = path.resolve(here, "..");
const repoRoot = path.resolve(pkgRoot, "..", "..");
const openapiPath = path.join(repoRoot, "api", "openapi", "openapi.yaml");

function bin(name) {
  const suffix = process.platform === "win32" ? ".cmd" : "";
  return path.join(pkgRoot, "node_modules", ".bin", name + suffix);
}

function generatedHeader(from) {
  return (
    "// GENERATED FILE — DO NOT EDIT.\n" +
    `// Source: ${from}\n` +
    "// Regenerate with `make gen` (api-contract-generation: Generation Chain).\n\n"
  );
}

function ensureDir(p) {
  fs.mkdirSync(p, { recursive: true });
}

// 1. openapi-typescript: `paths`/`components` types from openapi.yaml.
function generateClientTypes() {
  const outDir = path.join(pkgRoot, "src", "client");
  ensureDir(outDir);
  const outFile = path.join(outDir, "openapi-types.ts");
  execFileSync(bin("openapi-typescript"), [openapiPath, "-o", outFile], {
    stdio: "inherit",
  });
  const generated = fs.readFileSync(outFile, "utf8");
  fs.writeFileSync(outFile, generatedHeader("api/openapi/openapi.yaml") + generated);
}

// 2. A deterministic, generated one-line wrapper around openapi-fetch's
// createClient, typed against the generated `paths` interface. It defines
// no type of its own beyond re-exporting the generated one.
function generateClientFactory() {
  const outFile = path.join(pkgRoot, "src", "client", "index.ts");
  const contents =
    generatedHeader("api/openapi/openapi.yaml (via openapi-typescript)") +
    `import createClient from "openapi-fetch";
import type { Client, ClientOptions } from "openapi-fetch";
import type { paths } from "./openapi-types.js";

export type { paths } from "./openapi-types.js";
export type { ClientOptions } from "openapi-fetch";

/** Typed fetch client for the Vecingest API, generated from openapi.yaml. */
export function createApiClient(options?: ClientOptions): Client<paths> {
  return createClient<paths>(options);
}

export type ApiClient = Client<paths>;
`;
  fs.writeFileSync(outFile, contents);
}

// 3. openapi-zod-client, schemas-only template: pure Zod schemas, no
// zodios client scaffolding (the HTTP client is openapi-fetch, per D-K).
function generateZodSchemas() {
  const outDir = path.join(pkgRoot, "src", "schemas");
  ensureDir(outDir);
  const outFile = path.join(outDir, "index.ts");
  const template = path.join(
    pkgRoot,
    "node_modules",
    "openapi-zod-client",
    "src",
    "templates",
    "schemas-only.hbs",
  );
  execFileSync(
    bin("openapi-zod-client"),
    [openapiPath, "-o", outFile, "-t", template, "--export-schemas"],
    { stdio: "inherit" },
  );
  const generated = fs.readFileSync(outFile, "utf8");
  fs.writeFileSync(outFile, generatedHeader("api/openapi/openapi.yaml") + generated);
}

// 4. Deterministic barrel re-exporting the client, schemas and the
// hand-authored stable error codes (src/errors.ts is not generated).
function generateBarrel() {
  const outFile = path.join(pkgRoot, "src", "index.ts");
  const contents =
    generatedHeader("this script's own barrel wiring") +
    `export * from "./client/index.js";
export * from "./schemas/index.js";
export * from "./errors.js";
`;
  fs.writeFileSync(outFile, contents);
}

generateClientTypes();
generateClientFactory();
generateZodSchemas();
generateBarrel();

console.log("packages/shared: generation complete (client, schemas, barrel).");

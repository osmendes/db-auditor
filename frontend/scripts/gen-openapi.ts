import { readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

type SchemaProp = {
  type?: string | string[];
  $ref?: string;
  items?: SchemaProp;
};

type Schema = {
  required?: string[];
  properties?: Record<string, SchemaProp>;
};

const here = dirname(fileURLToPath(import.meta.url));
const specPath = resolve(here, "../../backend/api/openapi.json");
const outPath = resolve(here, "../src/types/openapi.ts");
const spec = JSON.parse(readFileSync(specPath, "utf8")) as {
  info: { version: string };
  paths: Record<string, unknown>;
  components?: { schemas?: Record<string, Schema> };
};

function refName(ref: string): string {
  const name = ref.split("/").pop() ?? "unknown";
  return name;
}

function tsType(prop: SchemaProp): string {
  if (prop.$ref) return refName(prop.$ref);
  if (prop.type === "array") {
    return `${tsType(prop.items ?? { type: "string" })}[]`;
  }
  if (Array.isArray(prop.type)) {
    return prop.type
      .map((item) => {
        if (item === "integer" || item === "number") return "number";
        if (item === "boolean") return "boolean";
        if (item === "null") return "null";
        if (item === "array") return `${tsType(prop.items ?? { type: "string" })}[]`;
        if (item === "object") return "Record<string, unknown>";
        return "string";
      })
      .join(" | ");
  }
  if (prop.type === "integer" || prop.type === "number") return "number";
  if (prop.type === "boolean") return "boolean";
  if (prop.type === "object") return "Record<string, unknown>";
  return "string";
}

const paths = Object.keys(spec.paths).sort();
const schemas = spec.components?.schemas ?? {};
const lines = [
  "/** Generated from backend/api/openapi.json. Do not edit. */",
  `export const apiContractVersion = ${JSON.stringify(spec.info.version)};`,
  "",
  "export type ApiPath =",
  ...paths.map(
    (path, index) =>
      `  | ${JSON.stringify(path)}${index === paths.length - 1 ? ";" : ""}`,
  ),
  "",
];

for (const name of Object.keys(schemas).sort()) {
  const schema = schemas[name];
  const required = new Set(schema.required ?? []);
  lines.push(`export interface ${name} {`);
  for (const [key, prop] of Object.entries(schema.properties ?? {})) {
    const optional = required.has(key) ? "" : "?";
    lines.push(`  ${key}${optional}: ${tsType(prop)};`);
  }
  lines.push("}", "");
}

writeFileSync(outPath, `${lines.join("\n").replace(/\n+$/, "\n")}`);

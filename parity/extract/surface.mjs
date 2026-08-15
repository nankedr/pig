// surface.mjs — Pi public-surface extractor for Milestone 0 (issue #21).
//
// Node + the locked TypeScript Compiler (typescript@5.9.3, pinned in
// package.json/package-lock.json) are used ONLY here and by the Pi Oracle;
// normal Pig build/test never runs this and stays pure-Go with no Node.
//
// It resolves each in-scope module's public entry points from package.json
// (main/types/exports + bin), walks the export surface through the TypeScript
// Compiler API, and emits one canonical JSONL record per unique exported symbol
// (deduped by declaration site) with its public members nested. The Go side
// (internal/surface) loads and validates this generated authority offline; it is
// the machine-readable surface view, parallel to parity/inventory/files.jsonl.
//
// Usage: node surface.mjs <pi-checkout-root> [--out <path>]
// Default out: ../surface/symbols.jsonl relative to this file.

import ts from "typescript";
import { readFileSync, existsSync, mkdirSync, writeFileSync } from "node:fs";
import { join, relative, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const SCHEMA_VERSION = "1.0.0";
const BASELINE_COMMIT = "936aff00918de1187f085f123c2812d8f2d67745";
const REPOSITORY = "https://github.com/badlogic/pi-mono";

const here = dirname(fileURLToPath(import.meta.url));

// The seven in-scope modules, in deterministic order. dir is relative to the Pi
// checkout root; pig is the Pig module bucket the symbols map into.
const MODULES = [
	{ dir: "packages/agent", pig: "agent" },
	{ dir: "packages/ai", pig: "ai" },
	{ dir: "packages/client", pig: "client" },
	{ dir: "packages/coding-agent", pig: "codingagent" },
	{ dir: "packages/protocol", pig: "protocol" },
	{ dir: "packages/telemetry", pig: "telemetry" },
	{ dir: "packages/tui", pig: "tui" },
];

function parseArgs(argv) {
	const args = { piRoot: null, out: join(here, "..", "surface", "symbols.jsonl") };
	const rest = argv.slice(2);
	for (let i = 0; i < rest.length; i++) {
		if (rest[i] === "--out") args.out = rest[++i];
		else if (!args.piRoot) args.piRoot = rest[i];
	}
	return args;
}

// distTargetToSrc maps a built dist target (e.g. "./dist/providers/openai.js")
// back to its TypeScript source under the module, mirroring the Go walker's
// resolution. Glob targets ("*") are expanded by the caller.
function distTargetToSrc(moduleAbs, target) {
	let rel = target
		.replace(/^\.\//, "")
		.replace(/^dist\//, "")
		.replace(/\.js$/, "")
		.replace(/\.d\.ts$/, "")
		.replace(/\.ts$/, "");
	if (!rel || rel === "package.json") return [];
	if (rel.includes("*")) {
		return []; // glob subpaths are resolved from the filesystem in resolveEntries
	}
	for (const cand of [`src/${rel}.ts`, `src/${rel}.tsx`, `${rel}.ts`, `${rel}.d.ts`]) {
		if (existsSync(join(moduleAbs, cand))) return [{ src: cand, subpath: rel }];
	}
	return [];
}

function collectExportEntries(exportsField) {
	// Returns [{subpath, target}] for every string target in the exports map.
	const out = [];
	const walk = (node, subpath) => {
		if (typeof node === "string") {
			out.push({ subpath, target: node });
		} else if (Array.isArray(node)) {
			node.forEach((v) => walk(v, subpath));
		} else if (node && typeof node === "object") {
			for (const [key, val] of Object.entries(node)) {
				// Keys beginning with "." are subpaths; condition keys (import,
				// types, ...) keep the current subpath.
				walk(val, key.startsWith(".") ? key : subpath);
			}
		}
	};
	walk(exportsField, ".");
	return out;
}

function collectBinTargets(bin) {
	if (!bin) return [];
	if (typeof bin === "string") return [bin];
	return Object.values(bin);
}

// resolveEntries returns the module's public entry source files, each tagged
// with the export subpath it backs and whether it is a bin (direct) entry.
function resolveEntries(moduleAbs) {
	const pkg = JSON.parse(readFileSync(join(moduleAbs, "package.json"), "utf8"));
	const entries = new Map(); // src -> {subpath, isBin}

	const add = (src, subpath, isBin) => {
		if (!entries.has(src)) entries.set(src, { subpath, isBin });
	};

	if (pkg.types) for (const e of distTargetToSrc(moduleAbs, pkg.types)) add(e.src, ".", false);
	if (pkg.main) for (const e of distTargetToSrc(moduleAbs, pkg.main)) add(e.src, ".", false);
	for (const { subpath, target } of collectExportEntries(pkg.exports || {})) {
		for (const e of distTargetToSrc(moduleAbs, target)) add(e.src, subpath, false);
	}
	for (const target of collectBinTargets(pkg.bin)) {
		for (const e of distTargetToSrc(moduleAbs, target)) add(e.src, "bin", true);
	}
	return entries;
}

function symbolKind(sym, checker) {
	const f = sym.getFlags();
	if (f & ts.SymbolFlags.Class) return "class";
	if (f & ts.SymbolFlags.Interface) return "interface";
	if (f & ts.SymbolFlags.TypeAlias) return "type";
	if (f & ts.SymbolFlags.Enum) return "enum";
	if (f & ts.SymbolFlags.ConstEnum) return "enum";
	if (f & ts.SymbolFlags.Function) return "function";
	if (f & ts.SymbolFlags.Method) return "method";
	if (f & ts.SymbolFlags.Namespace || f & ts.SymbolFlags.Module) return "namespace";
	if (f & ts.SymbolFlags.Variable || f & ts.SymbolFlags.BlockScopedVariable) return "const";
	return "value";
}

function unaliased(sym, checker) {
	if (sym.getFlags() & ts.SymbolFlags.Alias) {
		try {
			return checker.getAliasedSymbol(sym);
		} catch {
			return sym;
		}
	}
	return sym;
}

function declarationRef(sym, piRoot) {
	const decls = sym.getDeclarations && sym.getDeclarations();
	if (!decls || decls.length === 0) return null;
	const file = decls[0].getSourceFile().fileName;
	if (file.includes("node_modules")) return { external: true, file };
	return { external: false, file: relative(piRoot, file).split("\\").join("/") };
}

// memberNames returns the sorted public member names of a symbol's declared
// type (properties and methods), skipping private/internal (leading "_") names.
function memberNames(sym, checker) {
	let target = sym;
	try {
		const t = checker.getDeclaredTypeOfSymbol(target);
		if (!t || !t.getProperties) return [];
		const names = [];
		for (const p of t.getProperties()) {
			const name = p.getName();
			if (name.startsWith("_")) continue;
			names.push(name);
		}
		return [...new Set(names)].sort();
	} catch {
		return [];
	}
}

function main() {
	const { piRoot, out } = parseArgs(process.argv);
	if (!piRoot) {
		console.error("usage: node surface.mjs <pi-checkout-root> [--out <path>]");
		process.exit(2);
	}

	// One program over every module entry: shared type checker, cross-module
	// re-exports resolve to their true declaration site.
	const entryMeta = new Map(); // absolute src -> {pig, subpath, isBin}
	for (const module of MODULES) {
		const moduleAbs = join(piRoot, module.dir);
		for (const [src, meta] of resolveEntries(moduleAbs)) {
			const abs = join(moduleAbs, src);
			if (!entryMeta.has(abs)) entryMeta.set(abs, { pig: module.pig, module: module.dir, ...meta });
		}
	}
	const rootFiles = [...entryMeta.keys()];

	const program = ts.createProgram(rootFiles, {
		target: ts.ScriptTarget.ES2022,
		module: ts.ModuleKind.NodeNext,
		moduleResolution: ts.ModuleResolutionKind.NodeNext,
		allowImportingTsExtensions: true,
		resolveJsonModule: true,
		skipLibCheck: true,
		noEmit: true,
		strict: true,
	});
	const checker = program.getTypeChecker();

	// Deduplicate by declaration-file#name so a symbol re-exported from many
	// entry points is recorded once, against the module that owns its
	// declaration file.
	const bySymbol = new Map();

	for (const entryAbs of rootFiles) {
		const meta = entryMeta.get(entryAbs);
		const sf = program.getSourceFile(entryAbs);
		if (!sf) {
			console.error("missing source file:", entryAbs);
			continue;
		}
		const moduleSymbol = checker.getSymbolAtLocation(sf);
		if (!moduleSymbol) continue;
		for (const exp of checker.getExportsOfModule(moduleSymbol)) {
			const name = exp.getName();
			const resolved = unaliased(exp, checker);
			const ref = declarationRef(resolved, piRoot);
			if (!ref || ref.external) continue; // re-exported third-party symbol

			// Attribute to the owning module by declaration file prefix.
			const owner = MODULES.find((m) => ref.file.startsWith(m.dir + "/"));
			if (!owner) continue;

			const key = ref.file + "#" + name;
			let record = bySymbol.get(key);
			if (!record) {
				record = {
					schema_version: SCHEMA_VERSION,
					id: `symbol:${owner.pig}/${ref.file.slice(owner.dir.length + 1)}#${name}`,
					module: owner.pig,
					name,
					kind: symbolKind(resolved, checker),
					upstream: {
						module: owner.dir.replace("packages/", ""),
						repository: REPOSITORY,
						commit: BASELINE_COMMIT,
						reference: `${ref.file}#${name}`,
					},
					export_subpaths: new Set(),
					members: memberNames(resolved, checker),
				};
				bySymbol.set(key, record);
			}
			record.export_subpaths.add(meta.subpath);
		}
	}

	const records = [...bySymbol.values()]
		.map((r) => ({
			schema_version: r.schema_version,
			id: r.id,
			module: r.module,
			name: r.name,
			kind: r.kind,
			upstream: r.upstream,
			export_subpaths: [...r.export_subpaths].sort(),
			members: r.members,
		}))
		.sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));

	// Canonical JSONL: one compact object per line, sorted by id.
	const lines = records.map((r) => JSON.stringify(r)).join("\n") + "\n";
	mkdirSync(dirname(out), { recursive: true });
	writeFileSync(out, lines);

	const memberTotal = records.reduce((n, r) => n + r.members.length, 0);
	console.error(
		`surface: ${records.length} symbols across ${MODULES.length} modules, ${memberTotal} members -> ${out}`,
	);
}

main();

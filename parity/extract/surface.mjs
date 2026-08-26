// surface.mjs — Pi public-surface extractor for Milestone 0 (issue #21).
//
// Node + the locked TypeScript Compiler (typescript@5.9.3, pinned in
// package.json/package-lock.json) are used ONLY here and by the Pi Oracle;
// normal Pig build/test never runs this and stays pure-Go with no Node.
//
// It resolves each in-scope module's public entry points from package.json
// (main/types/exports + bin, wildcard export subpaths expanded, and — for
// packages with no "exports" gate — every technically reachable src deep
// import), walks the export surface through the TypeScript Compiler API, and
// emits one canonical JSONL record per unique exported symbol (deduped by
// declaration site) with its public members nested. The Go side
// (internal/surface) loads and validates this generated authority offline; it is
// the machine-readable surface view, parallel to parity/inventory/files.jsonl.
//
// Usage: node surface.mjs <pi-checkout-root> [--out <path>]
// Default out: ../surface/symbols.jsonl relative to this file.

import ts from "typescript";
import { readFileSync, readdirSync, existsSync, mkdirSync, writeFileSync } from "node:fs";
import { join, relative, dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const SCHEMA_VERSION = "1.1.0";
const BASELINE_COMMIT = "936aff00918de1187f085f123c2812d8f2d67745";
const REPOSITORY = "https://github.com/badlogic/pi-mono";

const here = dirname(fileURLToPath(import.meta.url));

// The seven in-scope modules, in deterministic order. dir is relative to the Pi
// checkout root; pig is the Pig module bucket the symbols map into.
const MODULES = [
	{ dir: "packages/agent", packageName: "@earendil-works/pi-agent-core", pig: "agent" },
	{ dir: "packages/ai", packageName: "@earendil-works/pi-ai", pig: "ai" },
	{ dir: "packages/client", packageName: "@earendil-works/pi-client", pig: "client" },
	{
		dir: "packages/coding-agent",
		packageName: "@earendil-works/pi-coding-agent",
		pig: "codingagent",
	},
	{ dir: "packages/protocol", packageName: "@earendil-works/pi-protocol", pig: "protocol" },
	{
		dir: "packages/telemetry",
		packageName: "@earendil-works/pi-telemetry",
		pig: "telemetry",
	},
	{ dir: "packages/tui", packageName: "@earendil-works/pi-tui", pig: "tui" },
];
const PI_PACKAGE_NAMES = new Set(MODULES.map((module) => module.packageName));

// Leading underscores are an internal-name convention throughout the pinned
// Pi surface, except for these exact TypeScript-public members. Keep the full
// surface identity here so an unrelated symbol cannot become public merely by
// reusing one of the allowed member names.
const ALLOWED_LEADING_UNDERSCORE_MEMBERS = new Set([
	"symbol:agent/src/harness/agent-harness.ts#AbortRejected._tag",
	"symbol:agent/src/harness/agent-harness.ts#CancelQueuedRejected._tag",
	"symbol:agent/src/harness/agent-harness.ts#Closed._tag",
	"symbol:agent/src/harness/agent-harness.ts#CompactionRejected._tag",
	"symbol:agent/src/harness/agent-harness.ts#InvalidLane._tag",
	"symbol:agent/src/harness/agent-harness.ts#InvalidMessage._tag",
	"symbol:agent/src/harness/agent-harness.ts#LaneBusy._tag",
	"symbol:agent/src/harness/agent-harness.ts#LaneExists._tag",
	"symbol:agent/src/harness/agent-harness.ts#MissingIdentities._tag",
	"symbol:agent/src/harness/agent-harness.ts#NavigationRejected._tag",
	"symbol:agent/src/harness/agent-harness.ts#NoActiveOperation._tag",
	"symbol:agent/src/harness/agent-harness.ts#NoActiveRun._tag",
	"symbol:agent/src/harness/agent-harness.ts#NothingToCompact._tag",
	"symbol:agent/src/harness/agent-harness.ts#NothingToResume._tag",
	"symbol:agent/src/harness/agent-harness.ts#QueueRejected._tag",
	"symbol:agent/src/harness/agent-harness.ts#ResumeRejected._tag",
	"symbol:agent/src/harness/agent-harness.ts#RunRejected._tag",
	"symbol:agent/src/harness/agent-harness.ts#UnknownQueueItem._tag",
	"symbol:agent/src/harness/agent-harness.ts#UnknownSkill._tag",
	"symbol:agent/src/harness/agent-harness.ts#UnknownTarget._tag",
	"symbol:agent/src/harness/agent-harness.ts#UnknownTemplate._tag",
	"symbol:agent/src/harness/result.ts#TaggedErrorValue._tag",
	"symbol:codingagent/src/core/session-manager.ts#SessionManager._persist",
]);

function parseArgs(argv) {
	const args = { piRoot: null, out: join(here, "..", "surface", "symbols.jsonl") };
	const rest = argv.slice(2);
	for (let i = 0; i < rest.length; i++) {
		if (rest[i] === "--out") args.out = rest[++i];
		else if (!args.piRoot) args.piRoot = rest[i];
	}
	return args;
}

function compilerOptions() {
	return {
		target: ts.ScriptTarget.ES2022,
		module: ts.ModuleKind.NodeNext,
		moduleResolution: ts.ModuleResolutionKind.NodeNext,
		allowImportingTsExtensions: true,
		resolveJsonModule: true,
		skipLibCheck: true,
		noEmit: true,
		strict: true,
	};
}

// loadWorkspaceCompilerOptions adds the frozen checkout's workspace paths to
// the extractor's stable compiler policy. A separate Program uses these options
// only for class member inheritance, so unrelated export/type extraction keeps
// its established semantics.
function loadWorkspaceCompilerOptions(piRoot) {
	const configPath = join(piRoot, "tsconfig.json");
	const read = ts.readConfigFile(configPath, ts.sys.readFile);
	if (read.error) {
		throw new Error(formatDiagnostic(read.error));
	}
	const parsed = ts.parseJsonConfigFileContent(read.config, ts.sys, piRoot, { noEmit: true }, configPath);
	if (parsed.errors.length > 0) {
		throw new Error(parsed.errors.map(formatDiagnostic).join("\n"));
	}
	return {
		...compilerOptions(),
		baseUrl: piRoot,
		paths: parsed.options.paths,
	};
}

function formatDiagnostic(diagnostic) {
	return ts.flattenDiagnosticMessageText(diagnostic.messageText, "\n");
}

// distTargetToSrc maps a built dist target (e.g. "./dist/providers/openai.js")
// back to its TypeScript source under the module, mirroring the Go walker's
// resolution. A wildcard target ("./dist/providers/*.js") is expanded against
// the source tree so every technically reachable deep-import file is covered.
function distTargetToSrc(moduleAbs, target) {
	let rel = target
		.replace(/^\.\//, "")
		.replace(/^dist\//, "")
		.replace(/\.js$/, "")
		.replace(/\.d\.ts$/, "")
		.replace(/\.ts$/, "");
	if (!rel || rel === "package.json") return [];
	if (rel.includes("*")) {
		return expandGlobSrc(moduleAbs, rel);
	}
	for (const cand of [`src/${rel}.ts`, `src/${rel}.tsx`, `${rel}.ts`, `${rel}.d.ts`]) {
		if (existsSync(join(moduleAbs, cand))) return [{ src: cand, subpath: rel }];
	}
	return [];
}

// expandGlobSrc resolves a single-"*" subpath target (e.g. "providers/*") to the
// matching source files under src/, so wildcard exports contribute their real
// public deep-import surface rather than nothing. Each result carries the "*"
// capture (star) so the caller can form the concrete public subpath.
function expandGlobSrc(moduleAbs, relPattern) {
	const star = relPattern.indexOf("*");
	if (star < 0 || relPattern.indexOf("*", star + 1) >= 0) return []; // only single "*"
	const prefix = relPattern.slice(0, star); // e.g. "providers/"
	const suffix = relPattern.slice(star + 1); // usually ""
	const dirRel = prefix.endsWith("/") ? prefix.slice(0, -1) : dirname(prefix);
	const dirAbs = join(moduleAbs, "src", dirRel);
	if (!existsSync(dirAbs)) return [];
	const out = [];
	for (const name of readdirSync(dirAbs)) {
		const cap = matchStar(name, prefix.slice(dirRel ? dirRel.length + 1 : 0), suffix);
		if (cap === null) continue;
		const src = join("src", dirRel, name).split("\\").join("/");
		out.push({ src, star: cap });
	}
	return out;
}

// matchStar returns the "*" capture for a filename against prefix+"*"+suffix,
// stripping a .ts/.tsx source extension, or null when it does not match a
// concrete TypeScript source (skips .d.ts and non-source files).
function matchStar(name, prefix, suffix) {
	let stem = name;
	if (stem.endsWith(".d.ts")) return null;
	if (stem.endsWith(".tsx")) stem = stem.slice(0, -4);
	else if (stem.endsWith(".ts")) stem = stem.slice(0, -3);
	else return null;
	if (!stem.startsWith(prefix) || !stem.endsWith(suffix)) return null;
	return stem.slice(prefix.length, stem.length - suffix.length);
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
//
// A package with no "exports" field is not encapsulated: Node lets consumers
// deep-import any built file, so every src module is a technically reachable
// public entry (this is how the TUI surface is consumed). Packages that declare
// "exports" are gated to their listed subpaths.
function resolveEntries(moduleAbs) {
	const pkg = JSON.parse(readFileSync(join(moduleAbs, "package.json"), "utf8"));
	const entries = new Map(); // src -> {subpath, isBin}

	const add = (src, subpath, isBin) => {
		if (!entries.has(src)) entries.set(src, { subpath, isBin });
	};

	if (pkg.types) for (const e of distTargetToSrc(moduleAbs, pkg.types)) add(e.src, ".", false);
	if (pkg.main) for (const e of distTargetToSrc(moduleAbs, pkg.main)) add(e.src, ".", false);
	for (const { subpath, target } of collectExportEntries(pkg.exports || {})) {
		for (const e of distTargetToSrc(moduleAbs, target)) {
			// Wildcard export keys ("./providers/*") resolve to a concrete
			// public subpath per matched file ("./providers/anthropic").
			const concrete = e.star !== undefined ? subpath.replace("*", e.star) : subpath;
			add(e.src, concrete, false);
		}
	}
	for (const target of collectBinTargets(pkg.bin)) {
		for (const e of distTargetToSrc(moduleAbs, target)) add(e.src, "bin", true);
	}
	if (!pkg.exports) {
		for (const { src, subpath } of walkSrcFiles(moduleAbs)) add(src, subpath, false);
	}
	return entries;
}

// walkSrcFiles returns every TypeScript source module under src/, each tagged
// with the deep-import subpath it backs (its src-relative path without the
// extension). Declaration files and test/story files are skipped: they are not
// public runtime deep-import targets. Used only for unencapsulated packages.
function walkSrcFiles(moduleAbs) {
	const srcAbs = join(moduleAbs, "src");
	if (!existsSync(srcAbs)) return [];
	const out = [];
	const walk = (dirAbs) => {
		for (const ent of readdirSync(dirAbs, { withFileTypes: true })) {
			const abs = join(dirAbs, ent.name);
			if (ent.isDirectory()) {
				walk(abs);
				continue;
			}
			if (ent.name.endsWith(".d.ts")) continue;
			if (/\.(test|spec|stories)\.tsx?$/.test(ent.name)) continue;
			if (!/\.tsx?$/.test(ent.name)) continue;
			const src = relative(moduleAbs, abs).split("\\").join("/");
			const subpath = src.replace(/^src\//, "").replace(/\.tsx?$/, "");
			out.push({ src, subpath });
		}
	};
	walk(srcAbs);
	return out;
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
// type (properties and methods). It skips members that are not part of the
// public surface: ECMAScript #private fields (name begins with "#"), the
// TypeScript "private" or "protected" modifiers, leading-underscore names
// outside the exact pinned exceptions above, compiler-internal computed names, and
// inherited class members supplied by an external dependency. Pi workspace
// packages remain owners of their declarations whether TypeScript resolves an
// import to checkout source or to that package beneath a node_modules tree.
function memberNames(sym, checker, program, piRoot, symbolID) {
	let target = sym;
	try {
		const t = checker.getDeclaredTypeOfSymbol(target);
		if (!t || !t.getProperties) return [];
		const names = [];
		const isClass = Boolean(sym.getFlags() & ts.SymbolFlags.Class);
		for (const p of t.getProperties()) {
			const name = p.getName();
			if (name.startsWith("#") || name.startsWith("__@")) continue;
			if (name.startsWith("_") && !ALLOWED_LEADING_UNDERSCORE_MEMBERS.has(`${symbolID}.${name}`)) {
				continue;
			}
			if (isPrivateMember(p)) continue;
			if (isClass && isExternalDependencyMember(p, program, piRoot)) continue;
			names.push(name);
		}
		return [...new Set(names)].sort();
	} catch {
		return [];
	}
}

// isExternalDependencyMember reports whether every declaration comes from a
// source outside the in-scope Pi packages. Checking exact package ownership,
// rather than treating node_modules as synonymous with external, retains Pi
// inheritance under both workspace-source and installed-package resolution.
// TypeScript's standard library remains part of the declared member surface
// even though the compiler itself may be installed beneath node_modules.
function isExternalDependencyMember(prop, program, piRoot) {
	const decls = prop.getDeclarations && prop.getDeclarations();
	return Boolean(
		decls &&
			decls.length > 0 &&
			decls.every((d) => {
				const sourceFile = d.getSourceFile();
				if (program.isSourceFileDefaultLibrary(sourceFile)) return false;
				const file = normalizePath(sourceFile.fileName);
				return !isPiOwnedDeclaration(file, piRoot);
			}),
	);
}

function isPiOwnedDeclaration(file, piRoot, installedPackage = nodeModulesPackageName(file)) {
	if (installedPackage !== null) return PI_PACKAGE_NAMES.has(installedPackage);
	const normalizedRoot = normalizePath(piRoot);
	return MODULES.some((module) => {
		const sourceRoot = `${normalizedRoot}/${module.dir}`;
		return file === sourceRoot || file.startsWith(`${sourceRoot}/`);
	});
}

function nodeModulesPackageName(file) {
	const parts = file.split("/");
	const index = parts.lastIndexOf("node_modules");
	if (index < 0 || index + 1 >= parts.length) return null;
	const first = parts[index + 1];
	if (!first.startsWith("@")) return first;
	if (index + 2 >= parts.length) return null;
	return `${first}/${parts[index + 2]}`;
}

function normalizePath(path) {
	return resolve(path).split("\\").join("/").replace(/\/$/, "");
}

function assertOwnershipClassifier(piRoot) {
	const cases = [
		[join(piRoot, "packages", "tui", "src", "keybindings.ts"), true],
		[join(piRoot, "node_modules", "@earendil-works", "pi-tui", "dist", "index.d.ts"), true],
		[
			join(
				piRoot,
				"node_modules",
				".pnpm",
				"@earendil-works+pi-tui@0.84.1",
				"node_modules",
				"@earendil-works",
				"pi-tui",
				"dist",
				"index.d.ts",
			),
			true,
		],
		[join(piRoot, "packages", "ai", "node_modules", "@types", "node", "events.d.ts"), false],
		[join(dirname(piRoot), "node_modules", "@types", "node", "events.d.ts"), false],
	];
	for (const [file, want] of cases) {
		const got = isPiOwnedDeclaration(normalizePath(file), piRoot);
		if (got !== want) throw new Error(`Pi ownership for ${file} = ${got}, want ${want}`);
	}
}

// staticMemberNames returns the sorted names of public constructor-side class
// properties declared by Pi. The constructor type includes inherited statics;
// requiring a Pi-owned declaration excludes Function/Error/Node statics while
// retaining statics inherited through another Pi class or factory. A set
// collapses method overload declarations and getter/setter pairs by name.
function staticMemberNames(sym, checker, piRoot) {
	if (!(sym.getFlags() & ts.SymbolFlags.Class)) return [];
	const decls = sym.getDeclarations && sym.getDeclarations();
	const location = sym.valueDeclaration || (decls && decls[0]);
	if (!location) return [];
	try {
		const type = checker.getTypeOfSymbolAtLocation(sym, location);
		if (!type || !type.getProperties) return [];
		const names = new Set();
		for (const property of type.getProperties()) {
			const name = property.getName();
			if (
				!name ||
				name.startsWith("#") ||
				name.startsWith("__@") ||
				name.startsWith("_") ||
				name === "prototype" ||
				Object.values(ts.InternalSymbolName).includes(name)
			) {
				continue;
			}
			if (isPrivateMember(property)) continue;
			const propertyDecls = property.getDeclarations && property.getDeclarations();
			if (
				!propertyDecls ||
				!propertyDecls.some((decl) =>
					isPiOwnedDeclaration(normalizePath(decl.getSourceFile().fileName), piRoot),
				)
			) {
				continue;
			}
			names.add(name);
		}
		return [...names].sort();
	} catch {
		return [];
	}
}

// isConstructibleClass reports whether an exported class can be instantiated
// directly by a consumer. Classes with no constructor declaration have an
// implicit public constructor. Explicit constructor overloads are considered as
// a group: a private or protected declaration makes the constructor surface
// inaccessible. Abstract classes are never directly constructible.
function isConstructibleClass(sym) {
	if (!(sym.getFlags() & ts.SymbolFlags.Class)) return false;
	const decls = sym.getDeclarations && sym.getDeclarations();
	if (!decls) return false;

	const classDecls = decls.filter((decl) =>
		ts.isClassDeclaration(decl) || ts.isClassExpression(decl),
	);
	if (classDecls.length === 0) return false;

	return classDecls.some((decl) => {
		if (ts.getCombinedModifierFlags(decl) & ts.ModifierFlags.Abstract) return false;
		const constructors = decl.members.filter(ts.isConstructorDeclaration);
		if (constructors.length === 0) return true;
		return constructors.every((constructor) => {
			const mods = ts.getCombinedModifierFlags(constructor);
			return !(mods & (ts.ModifierFlags.Private | ts.ModifierFlags.Protected));
		});
	});
}

function assertConstructorClassifier() {
	const source = ts.createSourceFile(
		"constructor-sentinels.ts",
		[
			"class Implicit {}",
			"class PublicExplicit { constructor() {} }",
			"class PublicOverloads { constructor(value: string); constructor(value: number); constructor(value: string | number) {} }",
			"class Private { private constructor() {} }",
			"class Protected { protected constructor() {} }",
			"abstract class Abstract {}",
		].join("\n"),
		ts.ScriptTarget.ES2022,
		true,
		ts.ScriptKind.TS,
	);
	const classes = new Map(
		source.statements
			.filter(ts.isClassDeclaration)
			.map((decl) => [decl.name.text, decl]),
	);
	for (const [name, want] of [
		["Implicit", true],
		["PublicExplicit", true],
		["PublicOverloads", true],
		["Private", false],
		["Protected", false],
		["Abstract", false],
	]) {
		const decl = classes.get(name);
		const got = isConstructibleClass({
			getFlags: () => ts.SymbolFlags.Class,
			getDeclarations: () => [decl],
		});
		if (got !== want) throw new Error(`constructor classification for ${name} = ${got}, want ${want}`);
	}
}

// assertRequiredMembers keeps the two ownership sentinels close to extraction:
// one must retain Pi-owned inheritance, while the other must reject inheritance
// from Node's EventEmitter. Failing here prevents an install-layout-dependent
// surface from being written as if it were authoritative.
function assertRequiredMembers(records, id, want) {
	const record = records.find((candidate) => candidate.id === id);
	if (!record) throw new Error(`required surface symbol missing: ${id}`);
	if (record.members.length !== want.length || record.members.some((name, i) => name !== want[i])) {
		throw new Error(`${id} members ${JSON.stringify(record.members)} != ${JSON.stringify(want)}`);
	}
}

function assertRequiredMember(records, id, want) {
	const record = records.find((candidate) => candidate.id === id);
	if (!record) throw new Error(`required surface symbol missing: ${id}`);
	if (!record.members.includes(want)) {
		throw new Error(`${id} members ${JSON.stringify(record.members)} do not include ${want}`);
	}
}

function assertAllowedLeadingUnderscoreMembers(records) {
	const got = records
		.flatMap((record) =>
			record.members
				.filter((name) => name.startsWith("_"))
				.map((name) => `${record.id}.${name}`),
		)
		.sort();
	const want = [...ALLOWED_LEADING_UNDERSCORE_MEMBERS].sort();
	if (got.length !== want.length || got.some((name, i) => name !== want[i])) {
		throw new Error(
			`leading-underscore surface members ${JSON.stringify(got)} != ${JSON.stringify(want)}`,
		);
	}
}

function assertRequiredStaticMembers(records, id, want) {
	const record = records.find((candidate) => candidate.id === id);
	if (!record) throw new Error(`required surface symbol missing: ${id}`);
	if (
		record.static_members.length !== want.length ||
		record.static_members.some((name, i) => name !== want[i])
	) {
		throw new Error(`${id} static members ${JSON.stringify(record.static_members)} != ${JSON.stringify(want)}`);
	}
}

function assertConstructorSurface(records) {
	const nonConstructibleClasses = records
		.filter((record) => record.kind === "class" && !record.constructible)
		.map((record) => record.id)
		.sort();
	const wantNonConstructibleClasses = [
		"symbol:agent/src/harness/agent-harness.ts#AgentHarness",
		"symbol:codingagent/src/client/remote-session.ts#RemoteSession",
		"symbol:codingagent/src/core/model-runtime.ts#ModelRuntime",
		"symbol:codingagent/src/core/session-manager.ts#SessionManager",
		"symbol:codingagent/src/core/settings-manager.ts#SettingsManager",
		"symbol:tui/src/components/stack.ts#Stack",
		"symbol:tui/src/tui.ts#TuiBase",
	].sort();
	if (
		nonConstructibleClasses.length !== wantNonConstructibleClasses.length ||
		nonConstructibleClasses.some((id, i) => id !== wantNonConstructibleClasses[i])
	) {
		throw new Error(
			`non-constructible classes ${JSON.stringify(nonConstructibleClasses)} != ${JSON.stringify(wantNonConstructibleClasses)}`,
		);
	}

	const constructorTotal = records.filter((record) => record.constructible).length;
	if (constructorTotal !== 110) {
		throw new Error(`constructible symbol count ${constructorTotal} != 110`);
	}
	const codingAgentConstructorTotal = records.filter(
		(record) => record.module === "codingagent" && record.constructible,
	).length;
	if (codingAgentConstructorTotal !== 38) {
		throw new Error(`codingagent constructible symbol count ${codingAgentConstructorTotal} != 38`);
	}
}

function assertMemberAbsent(records, id, forbidden) {
	const record = records.find((candidate) => candidate.id === id);
	if (!record) throw new Error(`required surface symbol missing: ${id}`);
	if (record.members.includes(forbidden)) {
		throw new Error(`${id} unexpectedly contains workspace-augmented member ${forbidden}`);
	}
}

function assertRecordAbsent(records, id) {
	if (records.some((candidate) => candidate.id === id)) {
		throw new Error(`${id} leaked through a cross-package re-export`);
	}
}

// isPrivateMember reports whether a property symbol is declared with the
// TypeScript "private" (or "protected") access modifier on any declaration.
function isPrivateMember(prop) {
	const decls = prop.getDeclarations && prop.getDeclarations();
	if (!decls) return false;
	for (const d of decls) {
		const mods = ts.getCombinedModifierFlags(d);
		if (mods & (ts.ModifierFlags.Private | ts.ModifierFlags.Protected)) return true;
	}
	return false;
}

function main() {
	const args = parseArgs(process.argv);
	if (!args.piRoot) {
		console.error("usage: node surface.mjs <pi-checkout-root> [--out <path>]");
		process.exit(2);
	}
	const piRoot = resolve(args.piRoot);
	const out = args.out;
	assertOwnershipClassifier(piRoot);
	assertConstructorClassifier();

	// Both programs cover every module entry. The primary checker preserves the
	// established export/type surface; the workspace checker deterministically
	// resolves Pi-owned class inheritance from checkout source.
	const entryMeta = new Map(); // absolute src -> {pig, subpath, isBin}
	for (const module of MODULES) {
		const moduleAbs = join(piRoot, module.dir);
		for (const [src, meta] of resolveEntries(moduleAbs)) {
			const abs = join(moduleAbs, src);
			if (!entryMeta.has(abs)) entryMeta.set(abs, { pig: module.pig, module: module.dir, ...meta });
		}
	}
	const rootFiles = [...entryMeta.keys()];

	const program = ts.createProgram(rootFiles, compilerOptions());
	const checker = program.getTypeChecker();
	const workspaceProgram = ts.createProgram(rootFiles, loadWorkspaceCompilerOptions(piRoot));
	const workspaceChecker = workspaceProgram.getTypeChecker();

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
				const symbolID = `symbol:${owner.pig}/${ref.file.slice(owner.dir.length + 1)}#${name}`;
				let memberSymbol = resolved;
				let memberChecker = checker;
				let memberProgram = program;
				if (resolved.getFlags() & ts.SymbolFlags.Class) {
					const workspaceSource = workspaceProgram.getSourceFile(entryAbs);
					const workspaceModule = workspaceSource && workspaceChecker.getSymbolAtLocation(workspaceSource);
					const workspaceExport =
						workspaceModule &&
						workspaceChecker.getExportsOfModule(workspaceModule).find((candidate) => candidate.getName() === name);
					if (workspaceExport) {
						memberSymbol = unaliased(workspaceExport, workspaceChecker);
						memberChecker = workspaceChecker;
						memberProgram = workspaceProgram;
					}
				}
				record = {
					schema_version: SCHEMA_VERSION,
					id: symbolID,
					module: owner.pig,
					name,
					kind: symbolKind(resolved, checker),
					constructible: isConstructibleClass(memberSymbol),
					upstream: {
						module: owner.dir.replace("packages/", ""),
						repository: REPOSITORY,
						commit: BASELINE_COMMIT,
						reference: `${ref.file}#${name}`,
					},
					export_subpaths: new Set(),
					members: memberNames(memberSymbol, memberChecker, memberProgram, piRoot, symbolID),
					static_members: staticMemberNames(memberSymbol, memberChecker, piRoot),
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
			constructible: r.constructible,
			upstream: r.upstream,
			export_subpaths: [...r.export_subpaths].sort(),
			members: r.members,
			static_members: r.static_members,
		}))
		.sort((a, b) => (a.id < b.id ? -1 : a.id > b.id ? 1 : 0));

	assertRequiredMembers(records, "symbol:codingagent/src/core/keybindings.ts#KeybindingsManager", [
		"getConflicts",
		"getDefinition",
		"getEffectiveConfig",
		"getKeys",
		"getResolvedBindings",
		"getUserBindings",
		"matches",
		"reload",
		"setUserBindings",
	]);
	assertRequiredMembers(records, "symbol:tui/src/stdin-buffer.ts#StdinBuffer", [
		"clear",
		"destroy",
		"flush",
		"getBuffer",
		"process",
	]);
	assertRequiredMembers(records, "symbol:agent/src/harness/agent-harness.ts#HarnessFault", [
		"cause",
		"message",
		"name",
		"stack",
	]);
	assertRequiredMember(records, "symbol:agent/src/harness/result.ts#TaggedErrorValue", "_tag");
	assertRequiredMember(records, "symbol:codingagent/src/core/session-manager.ts#SessionManager", "_persist");
	assertAllowedLeadingUnderscoreMembers(records);
	assertRequiredStaticMembers(records, "symbol:agent/src/harness/agent-harness.ts#AgentHarness", [
		"create",
	]);
	assertRequiredStaticMembers(records, "symbol:agent/src/harness/agent-harness.ts#LaneBusy", ["is"]);
	assertRequiredStaticMembers(records, "symbol:agent/src/harness/agent-harness.ts#HarnessFault", []);
	assertConstructorSurface(records);
	assertMemberAbsent(records, "symbol:tui/src/keybindings.ts#Keybindings", "app.clear");
	assertRecordAbsent(records, "symbol:telemetry/src/index.ts#TelemetrySpanAttributes");

	// Canonical JSONL: one compact object per line, sorted by id.
	const lines = records.map((r) => JSON.stringify(r)).join("\n") + "\n";
	mkdirSync(dirname(out), { recursive: true });
	writeFileSync(out, lines);

	const memberTotal = records.reduce((n, r) => n + r.members.length, 0);
	const staticMemberTotal = records.reduce((n, r) => n + r.static_members.length, 0);
	const constructorTotal = records.filter((r) => r.constructible).length;
	console.error(
		`surface: ${records.length} symbols across ${MODULES.length} modules, ${memberTotal} members, ${staticMemberTotal} static members, ${constructorTotal} constructors -> ${out}`,
	);
}

main();

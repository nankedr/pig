import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "grep-tool.json");

function canonical(value) {
 if (typeof value === "number" && value !== 0) {
  const [mantissa, exponent = "0"] = String(value).split("e");
  const [whole, fraction = ""] = mantissa.split(".");
  const digits = (whole + fraction).replace(/^(-?)0+/, "$1");
  const trimmed = digits.replace(/0+$/, "");
  const power = Number(exponent) - fraction.length + digits.length - trimmed.length;
  return JSON.rawJSON(trimmed + (power ? `e${power}` : ""));
 }
	if (Array.isArray(value)) return value.map(canonical);
	if (value && typeof value === "object") {
		return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0).map(([key, child]) => [key, canonical(child)]));
	}
	return value;
}

function caseDigest(value) {
	const projected = {
		schema_version: value.schema_version,
		id: value.id,
		catalog_id: value.catalog_id,
		surface: value.surface,
		input: canonical(value.input),
		observe: value.observe,
	};
	return `sha256:${createHash("sha256").update(JSON.stringify(projected)).digest("hex")}`;
}

function observationDigest(value) {
	const projected = {
		outcome: canonical(value.outcome),
		side_effects: value.side_effects.map((effect) => ({ kind: effect.kind, target: effect.target, detail: canonical(effect.detail) })),
	};
	return `sha256:${createHash("sha256").update(JSON.stringify(projected)).digest("hex")}`;
}

function parseArgs(argv) {
	const result = { pi: defaultPi, out: defaultOutput, check: false };
	for (let i = 2; i < argv.length; i++) {
		if (argv[i] === "--check") result.check = true;
		else if (argv[i] === "--out") result.out = argv[++i];
		else if (result.pi === defaultPi) result.pi = argv[i];
		else throw new Error(`unexpected argument: ${argv[i]}`);
	}
	return result;
}

async function main() {
 const args = parseArgs(process.argv);
 const lock = JSON.parse(readFileSync(join(root, "parity/baseline/upstream.lock.json"), "utf8"));
 if (execFileSync("git", ["-C", args.pi, "rev-parse", "HEAD"], {encoding:"utf8"}).trim() !== lock.upstream.commit) throw new Error("Pi checkout does not match Code Baseline");
 if (execFileSync("git", ["-C", args.pi, "status", "--porcelain", "--untracked-files=no"], {encoding:"utf8"}).trim()) throw new Error("Pi checkout has tracked changes");
 const reference = "packages/coding-agent/src/core/tools/grep.ts";
 const {createGrepTool, createGrepToolDefinition} = await import(pathToFileURL(join(args.pi, reference)).href);
 const rgVersion=execFileSync("rg",["--version"],{encoding:"utf8"}).split("\n")[0];
 process.env.PI_OFFLINE="1";
 const dir = mkdtempSync(join(tmpdir(), "pi-grep-tool-"));
 try {
  const input = {files:{
   "example.txt":"before\nmatch one\nafter\nmatch two\nlast\n",
   "unicode.txt":"你好🙂 MATCH\r\n猫咪 café\r\nSTRASSE straße\r\n",
   "literal.txt":"a.b\naXb\n--hidden\n",
   "long.txt":"x".repeat(501)+"\n"+"🙂".repeat(251)+"\n"+"x".repeat(499)+"🙂\n",
   "many.txt":("x".repeat(499)+"\n").repeat(120),
   "cr.txt":"before\rmatch\rafter\r\nmatch again\r\n",
   "empty.txt":"",
"huge.txt":"z".repeat(100000),
   ".git/HEAD":"ref: refs/heads/main\n",
   ".gitignore":"ignored.txt\n",
   "ignored.txt":"ignored-needle\n",
   ".hidden.txt":"hidden-needle\n",
   "sub/deep.ts":"glob-needle\n",
   "space name.txt":"space-needle\n",
   "binary.txt":"binary-needle\u0000end\n"
  }, searches:[
   {pattern:"match",path:"example.txt"},
   {pattern:"match",path:"example.txt",context:1,readMode:"remote"},
   {pattern:"match",path:"example.txt",context:1,readMode:"denied"},
   {pattern:"match",path:"example.txt",context:1,limit:1},
   {pattern:"match",path:"example.txt",context:2},
   {pattern:"match",path:"example.txt",context:1.5,limit:1.5},
   {pattern:"match",path:"example.txt",context:-1,limit:0},
   {pattern:"match",path:"example.txt",limit:-3},
   {pattern:"MATCH",path:"unicode.txt",ignoreCase:true},
   {pattern:"猫咪|café",path:"unicode.txt"},
   {pattern:"a.b",path:"literal.txt",literal:true},
   {pattern:"a.b",path:"literal.txt"},
   {pattern:"--hidden",path:"literal.txt",literal:true},
   {pattern:"--pre=./payload.sh",path:"literal.txt"},
   {pattern:"absent",path:"example.txt"},
   {pattern:"",path:"empty.txt"},
{pattern:"z",path:"huge.txt"},
{pattern:"match",path:"example.txt",context:1e100,limit:1e100},
   {pattern:"",path:"example.txt"},
   {pattern:".",path:"long.txt"},
   {pattern:"x",path:"many.txt",limit:120},
   {pattern:"x",path:"many.txt"},
   {pattern:"match",path:"cr.txt"},
   {pattern:"match",path:"cr.txt",context:1},
   {pattern:"ignored-needle"},
   {pattern:"ignored-needle",path:"ignored.txt"},
   {pattern:"hidden-needle",path:""},
   {pattern:"glob-needle",glob:"**/*.ts"},
   {pattern:"glob-needle",glob:"*.txt"},
   {pattern:"space-needle",path:"@space\u202fname.txt"},
   {pattern:"binary-needle",path:"binary.txt"},
   {pattern:"match",path:"missing"},
   {pattern:"[",path:"example.txt"}
  ]};
  for (const [file, content] of Object.entries(input.files)) {
   mkdirSync(dirname(join(dir,file)),{recursive:true});
   writeFileSync(join(dir,file),content);
  }
  const tool=createGrepTool(dir);
  const results=[];
  for (const search of input.searches) {
   try {
    const selected=search.readMode ? createGrepTool(dir,{operations:{isDirectory:p=>statSync(p).isDirectory(),readFile:async()=>{if(search.readMode==="denied")throw new Error("denied");return "remote before\nremote match\nremote after";}}}) : tool;
    const result=await selected.execute("grep",search);
    results.push({text:Buffer.from(result.content[0].text).toString(),details:result.details??null,isError:false});
   } catch(error) {results.push({text:error.message.replaceAll(dir,"<cwd>"),details:null,isError:true});}
  }
  const definition=createGrepToolDefinition(dir);
  const metadata={name:definition.name,label:definition.label,description:definition.description,parameters:definition.parameters,promptSnippet:definition.promptSnippet};
  const observation={outcome:{results,metadata},side_effects:[]};
  const caseValue={schema_version:"1.0.0",id:"go-sdk/codingagent/grep-tool",catalog_id:"contract:codingagent/grep-tool",surface:"go-sdk",input,observe:["outcome","side_effects"]};
  const fixture={schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:caseValue,observation,input_hash:caseDigest(caseValue),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/grep-tool.mjs <locked-pi-checkout>",platform:"any",environment:{node:process.version,ripgrep:rgVersion,oracle_entry:reference}};
  if(args.check){const committed=JSON.parse(readFileSync(args.out,"utf8"));fixture.environment.node=committed.environment.node;if(JSON.stringify(fixture)!==JSON.stringify(committed))throw new Error("committed fixture does not reproduce");console.log(`verified ${args.out}`);}else{writeFileSync(args.out,`${JSON.stringify(fixture,null,2)}\n`);console.log(`wrote ${args.out}`);}
 } finally {rmSync(dir,{recursive:true,force:true});}
}
main().catch(error=>{console.error(error);process.exit(1);});

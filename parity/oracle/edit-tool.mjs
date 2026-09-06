import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "edit-tool.json");

function canonical(value) {
	if (Number.isInteger(value) && value !== 0) {
		const digits = String(value), trimmed = digits.replace(/0+$/, "");
		if (trimmed.length !== digits.length) return JSON.rawJSON(`${trimmed}e${digits.length-trimmed.length}`);
	}
	if (Array.isArray(value)) return value.map(canonical);
	if (value && typeof value === "object") {
		return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, child]) => [key, canonical(child)]));
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
 if(process.versions.unicode!=="16.0") throw new Error("edit Oracle requires Node with Unicode 16.0");
 const lock = JSON.parse(readFileSync(join(root, "parity/baseline/upstream.lock.json"), "utf8"));
 if (execFileSync("git", ["-C", args.pi, "rev-parse", "HEAD"], {encoding:"utf8"}).trim() !== lock.upstream.commit) throw new Error("Pi checkout does not match Code Baseline");
 if (execFileSync("git", ["-C", args.pi, "status", "--porcelain", "--untracked-files=no"], {encoding:"utf8"}).trim()) throw new Error("Pi checkout has tracked changes");
 const reference = "packages/coding-agent/src/core/tools/edit.ts";
 const {createEditToolDefinition} = await import(pathToFileURL(join(args.pi, reference)).href);
 const {createReadTool} = await import(pathToFileURL(join(args.pi, "packages/coding-agent/src/core/tools/read.ts")).href);
 const dir = mkdtempSync(join(tmpdir(), "pi-edit-tool-"));
 try {
  const input = {cases:[
{"name": "legacy", "content": "before\n", "args": {"oldText": "before", "newText": "after"}},
{"name": "legacy-appended", "content": "a\nc\n", "args": {"edits": [{"oldText": "a", "newText": "b"}], "oldText": "c", "newText": "d"}},
{"name": "stringified", "content": "before\n", "args": {"edits": "[{\"oldText\": \"before\", \"newText\": \"after\"}]"}},
{"name": "stringified-legacy", "content": "a\nc\n", "args": {"edits": "[{\"oldText\": \"a\", \"newText\": \"b\"}]", "oldText": "c", "newText": "d"}},
   {name:"exact",content:"Hello, world!",args:{edits:[{oldText:"world",newText:"testing"}]}},
{"name": "multi-original", "content": "foo\nbar\nbaz\n", "args": {"edits": [{"oldText": "foo\n", "newText": "foo bar\n"}, {"oldText": "bar\n", "newText": "BAR\n"}]}},
{"name": "multi-reverse", "content": "alpha\nbeta\ngamma\ndelta\n", "args": {"edits": [{"oldText": "gamma\n", "newText": "GAMMA\n"}, {"oldText": "alpha\n", "newText": "ALPHA\n"}]}},
{"name": "missing", "content": "hello\n", "args": {"edits": [{"oldText": "absent", "newText": "new"}]}},
{"name": "duplicate", "content": "foo foo foo", "args": {"edits": [{"oldText": "foo", "newText": "bar"}]}},
{"name": "empty-edits", "content": "hello\n", "args": {"edits": []}},
{"name": "empty-old", "content": "hello\n", "args": {"edits": [{"oldText": "", "newText": "x"}]}},
{"name": "empty-file", "content": "", "args": {"edits": [{"oldText": "hello", "newText": "x"}]}},
{"name": "empty-file-old", "content": "", "args": {"edits": [{"oldText": "", "newText": "x"}]}},
{"name": "no-change", "content": "hello\n", "args": {"edits": [{"oldText": "hello", "newText": "hello"}]}},
{"name": "multi-no-change", "content": "foo\nbar\n", "args": {"edits": [{"oldText": "foo", "newText": "foo"}, {"oldText": "bar", "newText": "bar"}]}},
{"name": "multi-empty-old", "content": "foo\nbar\n", "args": {"edits": [{"oldText": "foo", "newText": "x"}, {"oldText": "", "newText": "x"}]}},
{"name": "overlap", "content": "one\ntwo\nthree\n", "args": {"edits": [{"oldText": "one\ntwo\n", "newText": "ONE\nTWO\n"}, {"oldText": "two\nthree\n", "newText": "TWO\nTHREE\n"}]}},
{"name": "nested-reverse", "content": "one two three", "args": {"edits": [{"oldText": "two", "newText": "TWO"}, {"oldText": "one two", "newText": "ONE TWO"}]}},
{"name": "multi-failure-no-partial", "content": "alpha\nbeta\ngamma\n", "args": {"edits": [{"oldText": "alpha\n", "newText": "ALPHA\n"}, {"oldText": "missing\n", "newText": "MISSING\n"}]}},
{"name": "multi-duplicate", "content": "alpha\nbeta beta\n", "args": {"edits": [{"oldText": "alpha", "newText": "ALPHA"}, {"oldText": "beta", "newText": "BETA"}]}},
{"name": "non-overlap-count", "content": "aaa", "args": {"edits": [{"oldText": "aa", "newText": "x"}]}},
{"name": "delete-entire-file", "content": "hello\n", "args": {"edits": [{"oldText": "hello\n", "newText": ""}]}},
{"name": "remove-final-newline", "content": "hello\n", "args": {"edits": [{"oldText": "hello\n", "newText": "hello"}]}},
{"name": "add-final-newline", "content": "hello", "args": {"edits": [{"oldText": "hello", "newText": "hello\n"}]}},
{"name": "crlf-bom", "content": "﻿first\r\nsecond\r\nthird\r\nfourth\r\n", "args": {"edits": [{"oldText": "second\n", "newText": "SECOND\n"}, {"oldText": "fourth\n", "newText": "FOURTH\n"}]}},
{"name": "mixed-first-crlf", "content": "one\r\ntwo\nthree\r", "args": {"edits": [{"oldText": "two", "newText": "TWO"}]}},
{"name": "mixed-first-lf", "content": "one\ntwo\r\nthree\r", "args": {"edits": [{"oldText": "two\r", "newText": "TWO\r"}]}},
{"name": "cr-only", "content": "one\rtwo\r", "args": {"edits": [{"oldText": "two\r", "newText": "TWO\r"}]}},
{"name": "newline-duplicate", "content": "hello\r\nworld\r\n---\r\nhello\nworld\n", "args": {"edits": [{"oldText": "hello\nworld\n", "newText": "x\n"}]}},
{"name": "whitespace", "content": "line one   \nline two  \nline three\n", "args": {"edits": [{"oldText": "line one\nline two\n", "newText": "replaced\n"}]}},
{"name": "nfkc", "content": "你好，世界\n你好（世界）\nＡＢＣ１２３\ncafé\n", "args": {"edits": [{"oldText": "你好,世界\n你好(世界)\nABC123\ncafé\n", "newText": "你好，pi\n"}]}},
{"name": "quotes-dashes-spaces", "content": "keep “smart”  \nconsole.log(‘hello’);\n“world”\nrange: 1–5\nbreak—here\nhello world\nkeep after  \n", "args": {"edits": [{"oldText": "console.log('hello');\n\"world\"\nrange: 1-5\nbreak-here\nhello world\n", "newText": "replaced\n"}]}},
{"name": "exact-preserves-line", "content": "smart “quotes” and target   \n", "args": {"edits": [{"oldText": "target", "newText": "changed"}]}},
{"name": "fuzzy-duplicates", "content": "hello world   \nhello world\n", "args": {"edits": [{"oldText": "hello world", "newText": "replaced"}]}},
{"name": "fuzzy-multi", "content": "keep before  \nfirst target  \nfirst after\nkeep middle   \nsecond target  \nsecond after\nkeep after  \n", "args": {"edits": [{"oldText": "first target\nfirst after", "newText": "FIRST\nFIRST2"}, {"oldText": "second target\nsecond after", "newText": "SECOND\nSECOND2"}]}},
{"name": "fuzzy-duplicate-neighbor", "content": "replace me   \nafter   \n", "args": {"edits": [{"oldText": "replace me\n", "newText": "after\n"}]}},
{"name": "fuzzy-shared-line", "content": "Ａ foo   \nbar  \n", "args": {"edits": [{"oldText": "A", "newText": "Z"}, {"oldText": "foo", "newText": "FOO"}]}},
{"name": "emoji-offsets", "content": "🙂你好\nＡＢＣ target\nend\n", "args": {"edits": [{"oldText": "ABC", "newText": "XYZ"}, {"oldText": "🙂你好", "newText": "🙃世界"}]}},
{"name": "fuzzy-empty-old", "content": "a  \n", "args": {"edits": [{"oldText": "  ", "newText": "b"}]}},
{"name": "fuzzy-all-whitespace", "content": "   ", "args": {"edits": [{"oldText": " \t", "newText": "x"}]}},
{"name": "trim-ecmascript", "content": "first\nsecond﻿\n", "args": {"edits": [{"oldText": "first\nsecond\n", "newText": "done\n"}]}},
{"name": "diff-large-gap", "content": "line 001\nline 002\nline 003\nline 004\nline 005\nline 006\nline 007\nline 008\nline 009\nline 010\nline 011\nline 012\nline 013\nline 014\nline 015\nline 016\nline 017\nline 018\nline 019\nline 020\nline 021\nline 022\nline 023\nline 024\nline 025\nline 026\nline 027\nline 028\nline 029\nline 030\nline 031\nline 032\nline 033\nline 034\nline 035\nline 036\nline 037\nline 038\nline 039\nline 040\nline 041\nline 042\nline 043\nline 044\nline 045\nline 046\nline 047\nline 048\nline 049\nline 050\nline 051\nline 052\nline 053\nline 054\nline 055\nline 056\nline 057\nline 058\nline 059\nline 060\nline 061\nline 062\nline 063\nline 064\nline 065\nline 066\nline 067\nline 068\nline 069\nline 070\nline 071\nline 072\nline 073\nline 074\nline 075\nline 076\nline 077\nline 078\nline 079\nline 080\nline 081\nline 082\nline 083\nline 084\nline 085\nline 086\nline 087\nline 088\nline 089\nline 090\nline 091\nline 092\nline 093\nline 094\nline 095\nline 096\nline 097\nline 098\nline 099\nline 100\nline 101\nline 102\nline 103\nline 104\nline 105\nline 106\nline 107\nline 108\nline 109\nline 110\nline 111\nline 112\nline 113\nline 114\nline 115\nline 116\nline 117\nline 118\nline 119\nline 120\nline 121\nline 122\nline 123\nline 124\nline 125\nline 126\nline 127\nline 128\nline 129\nline 130\nline 131\nline 132\nline 133\nline 134\nline 135\nline 136\nline 137\nline 138\nline 139\nline 140\nline 141\nline 142\nline 143\nline 144\nline 145\nline 146\nline 147\nline 148\nline 149\nline 150\nline 151\nline 152\nline 153\nline 154\nline 155\nline 156\nline 157\nline 158\nline 159\nline 160\nline 161\nline 162\nline 163\nline 164\nline 165\nline 166\nline 167\nline 168\nline 169\nline 170\nline 171\nline 172\nline 173\nline 174\nline 175\nline 176\nline 177\nline 178\nline 179\nline 180\nline 181\nline 182\nline 183\nline 184\nline 185\nline 186\nline 187\nline 188\nline 189\nline 190\nline 191\nline 192\nline 193\nline 194\nline 195\nline 196\nline 197\nline 198\nline 199\nline 200\nline 201\nline 202\nline 203\nline 204\nline 205\nline 206\nline 207\nline 208\nline 209\nline 210\nline 211\nline 212\nline 213\nline 214\nline 215\nline 216\nline 217\nline 218\nline 219\nline 220\nline 221\nline 222\nline 223\nline 224\nline 225\nline 226\nline 227\nline 228\nline 229\nline 230\nline 231\nline 232\nline 233\nline 234\nline 235\nline 236\nline 237\nline 238\nline 239\nline 240\nline 241\nline 242\nline 243\nline 244\nline 245\nline 246\nline 247\nline 248\nline 249\nline 250\nline 251\nline 252\nline 253\nline 254\nline 255\nline 256\nline 257\nline 258\nline 259\nline 260\nline 261\nline 262\nline 263\nline 264\nline 265\nline 266\nline 267\nline 268\nline 269\nline 270\nline 271\nline 272\nline 273\nline 274\nline 275\nline 276\nline 277\nline 278\nline 279\nline 280\nline 281\nline 282\nline 283\nline 284\nline 285\nline 286\nline 287\nline 288\nline 289\nline 290\nline 291\nline 292\nline 293\nline 294\nline 295\nline 296\nline 297\nline 298\nline 299\nline 300\nline 301\nline 302\nline 303\nline 304\nline 305\nline 306\nline 307\nline 308\nline 309\nline 310\nline 311\nline 312\nline 313\nline 314\nline 315\nline 316\nline 317\nline 318\nline 319\nline 320\nline 321\nline 322\nline 323\nline 324\nline 325\nline 326\nline 327\nline 328\nline 329\nline 330\nline 331\nline 332\nline 333\nline 334\nline 335\nline 336\nline 337\nline 338\nline 339\nline 340\nline 341\nline 342\nline 343\nline 344\nline 345\nline 346\nline 347\nline 348\nline 349\nline 350\nline 351\nline 352\nline 353\nline 354\nline 355\nline 356\nline 357\nline 358\nline 359\nline 360\nline 361\nline 362\nline 363\nline 364\nline 365\nline 366\nline 367\nline 368\nline 369\nline 370\nline 371\nline 372\nline 373\nline 374\nline 375\nline 376\nline 377\nline 378\nline 379\nline 380\nline 381\nline 382\nline 383\nline 384\nline 385\nline 386\nline 387\nline 388\nline 389\nline 390\nline 391\nline 392\nline 393\nline 394\nline 395\nline 396\nline 397\nline 398\nline 399\nline 400\nline 401\nline 402\nline 403\nline 404\nline 405\nline 406\nline 407\nline 408\nline 409\nline 410\nline 411\nline 412\nline 413\nline 414\nline 415\nline 416\nline 417\nline 418\nline 419\nline 420\nline 421\nline 422\nline 423\nline 424\nline 425\nline 426\nline 427\nline 428\nline 429\nline 430\nline 431\nline 432\nline 433\nline 434\nline 435\nline 436\nline 437\nline 438\nline 439\nline 440\nline 441\nline 442\nline 443\nline 444\nline 445\nline 446\nline 447\nline 448\nline 449\nline 450\nline 451\nline 452\nline 453\nline 454\nline 455\nline 456\nline 457\nline 458\nline 459\nline 460\nline 461\nline 462\nline 463\nline 464\nline 465\nline 466\nline 467\nline 468\nline 469\nline 470\nline 471\nline 472\nline 473\nline 474\nline 475\nline 476\nline 477\nline 478\nline 479\nline 480\nline 481\nline 482\nline 483\nline 484\nline 485\nline 486\nline 487\nline 488\nline 489\nline 490\nline 491\nline 492\nline 493\nline 494\nline 495\nline 496\nline 497\nline 498\nline 499\nline 500\nline 501\nline 502\nline 503\nline 504\nline 505\nline 506\nline 507\nline 508\nline 509\nline 510\nline 511\nline 512\nline 513\nline 514\nline 515\nline 516\nline 517\nline 518\nline 519\nline 520\nline 521\nline 522\nline 523\nline 524\nline 525\nline 526\nline 527\nline 528\nline 529\nline 530\nline 531\nline 532\nline 533\nline 534\nline 535\nline 536\nline 537\nline 538\nline 539\nline 540\nline 541\nline 542\nline 543\nline 544\nline 545\nline 546\nline 547\nline 548\nline 549\nline 550\nline 551\nline 552\nline 553\nline 554\nline 555\nline 556\nline 557\nline 558\nline 559\nline 560\nline 561\nline 562\nline 563\nline 564\nline 565\nline 566\nline 567\nline 568\nline 569\nline 570\nline 571\nline 572\nline 573\nline 574\nline 575\nline 576\nline 577\nline 578\nline 579\nline 580\nline 581\nline 582\nline 583\nline 584\nline 585\nline 586\nline 587\nline 588\nline 589\nline 590\nline 591\nline 592\nline 593\nline 594\nline 595\nline 596\nline 597\nline 598\nline 599\nline 600\n", "args": {"edits": [{"oldText": "line 100\n", "newText": "LINE 100\n"}, {"oldText": "line 300\n", "newText": "LINE 300\n"}, {"oldText": "line 500\n", "newText": "LINE 500\n"}]}},
{"name": "diff-repeated-lines", "content": "a\nb\na\nb\nz\n", "args": {"edits": [{"oldText": "a\nb\na\nb\nz\n", "newText": "b\na\nb\na\nz\n"}]}},
{"name": "diff-gap-eight", "content": "start\n0\n1\n2\n3\n4\n5\n6\n7\nend\n", "args": {"edits": [{"oldText": "start", "newText": "START\nextra"}, {"oldText": "end", "newText": "END"}]}}
  ]};
  input.cases.push({name:"invalid-utf8",bytes:[0xef,0xbb,0xbf,0xe2,0x82,0x0a,0x74,0x61,0x72,0x67,0x65,0x74],args:{edits:[{oldText:"target",newText:"done"}]}});
  for(const [name,content,oldText] of [
   ["unicode16-outlined","\u{1CCD6}\n","A"],
   ["unicode16-ccc","A\u{1E5EE}\u{1E5EF}\n","A\u{1E5EF}\u{1E5EE}"],
   ["unicode16-composition","\u{105D2}\u0307\n","\u{105C9}"],
   ["unicode16-ccc-zero-composition","\u{1611E}\u{1611E}\u{1611F}\n","\u{16126}"],
   ["long-combining","Ａ"+"\u0315".repeat(35)+"\u0300\n","À"+"\u0315".repeat(35)],
   ["long-combining-cgj","Ａ"+"\u0315".repeat(31)+"\u034f\u0300\n","A"+"\u0315".repeat(31)+"\u034f\u0300"],
   ["hangul-composition","\u1100\u1161\u11a8\n","각"],
   ["combining-blocked","Ａ\u0305\u0300\n","A\u0305\u0300"]
  ]) input.cases.push({name,content,args:{edits:[{oldText,newText:"done"}]}});
  let seed = 79;
  const next = () => (seed = (Math.imul(seed, 1664525) + 1013904223) >>> 0);
  for(let i=0;i<32;i++) {
   const lines=Array.from({length:12+next()%24},()=>["a","b","c","","你好🙂"][next()%5]);
   const content=lines.join("\n")+(i%2?"\n":"");
   const changed=[...lines];
   changed.splice(next()%changed.length,1+next()%4,...Array.from({length:next()%5},()=>["a","b","","new"][next()%4]));
   const newText=changed.join("\n")+(i%3?"\n":"");
   input.cases.push({name:`diff-path-${i}`,content,args:{edits:[{oldText:content,newText}]}});
  }
  const definition=createEditToolDefinition(dir), read=createReadTool(dir);
  const results=[];
  for (const item of input.cases) {
   const path="target.txt";
   writeFileSync(join(dir,path),item.bytes ? Buffer.from(item.bytes) : item.content);
   let result;
   try {result=await definition.execute("edit",definition.prepareArguments({path,...structuredClone(item.args)}));}
   catch(error){result={content:[{type:"text",text:error.message}],details:{},isError:true};}
   const back=await read.execute("read",{path});
   results.push({text:result.content[0].text,details:result.details??null,isError:result.isError??false,read:back.content[0].text,content:readFileSync(join(dir,path),"utf8")});
  }
  const metadata={name:definition.name,label:definition.label,description:definition.description,parameters:definition.parameters,promptSnippet:definition.promptSnippet,promptGuidelines:definition.promptGuidelines};
  const observation={outcome:{results,metadata},side_effects:[]};
  const caseValue={schema_version:"1.0.0",id:"go-sdk/codingagent/edit-tool",catalog_id:"contract:codingagent/edit-tool",surface:"go-sdk",input,observe:["outcome","side_effects"]};
  const fixture={schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:caseValue,observation,input_hash:caseDigest(caseValue),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/edit-tool.mjs <locked-pi-checkout>",platform:"any",environment:{node:process.version,oracle_entry:reference}};
  if(args.check){const committed=JSON.parse(readFileSync(args.out,"utf8"));fixture.environment.node=committed.environment.node;if(JSON.stringify(fixture)!==JSON.stringify(committed))throw new Error("committed fixture does not reproduce");console.log(`verified ${args.out}`);}else{writeFileSync(args.out,`${JSON.stringify(fixture,null,2)}\n`);console.log(`wrote ${args.out}`);}
 } finally {rmSync(dir,{recursive:true,force:true});}
}
main().catch(error=>{console.error(error);process.exit(1);});

import { join } from "node:path";
import { pathToFileURL } from "node:url";

const piRoot = process.argv[2];
if (!piRoot) throw new Error("usage: export-image-models.mjs <pi-checkout-root>");

const source = join(piRoot, "packages", "ai", "src", "image-models.generated.ts");
const { IMAGE_MODELS } = await import(pathToFileURL(source).href);
process.stdout.write(`${JSON.stringify(IMAGE_MODELS)}\n`);

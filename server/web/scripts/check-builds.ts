// Fails when the web build holds code only the packaged TV app runs:
// its bearer header, the events socket's token subprotocol,
// its kept server and client ID, or its page's security policy. Fails too
// when it holds Norigin, which only the TV app runs. The flags are
// build-time constants, so the web build drops that code; a check written
// in a way the bundler can't fold would keep it. npm run build runs this
// after the build.
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

const packagedMarkers = ["kivo-bearer", "Bearer ", "kivo.server", "kivo.clientId", "Content-Security-Policy"];
// Norigin's root focus key.
const noriginMarkers = ["SN:ROOT"];

const dist = fileURLToPath(new URL("../dist", import.meta.url));

const web = [
  join(dist, "index.html"),
  ...readdirSync(join(dist, "assets"))
    .filter((name) => name.endsWith(".js"))
    .map((name) => join(dist, "assets", name)),
];

function found(files: string[], markers: string[]): string[] {
  return files.flatMap((file) => {
    const text = readFileSync(file, "utf8");
    return markers.filter((marker) => text.includes(marker)).map((marker) => `${file}: ${JSON.stringify(marker)}`);
  });
}

const packaged = found(web, packagedMarkers);
const norigin = found(web, noriginMarkers);

if (packaged.length > 0) console.error(`The web build holds packaged-app code:\n${packaged.join("\n")}`);
if (norigin.length > 0) console.error(`The web build holds Norigin:\n${norigin.join("\n")}`);
if (packaged.length > 0 || norigin.length > 0) process.exit(1);

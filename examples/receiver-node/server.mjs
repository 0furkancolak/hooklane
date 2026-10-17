// Minimal Hooklane webhook receiver for Node.js 20+, no dependencies.
//   HOOKLANE_SECRET=whsec_... node server.mjs   (HOST and PORT are optional)
import { createServer } from "node:http";
import { verify } from "./verify.mjs";

const secret = process.env.HOOKLANE_SECRET;
if (!secret) {
  console.error("HOOKLANE_SECRET is required (the endpoint's whsec_ secret)");
  process.exit(1);
}
const port = Number(process.env.PORT ?? 9000);
const host = process.env.HOST ?? "127.0.0.1";

const seen = new Set(); // use a database with a unique constraint in production

createServer((req, res) => {
  if (req.method !== "POST") {
    res.writeHead(405).end();
    return;
  }
  const chunks = [];
  let size = 0;
  req.on("data", (chunk) => {
    size += chunk.length;
    if (size > 1 << 20) req.destroy();
    else chunks.push(chunk);
  });
  req.on("end", () => {
    const raw = Buffer.concat(chunks);
    if (!verify(secret, req.headers, raw)) {
      res.writeHead(401).end("invalid signature");
      return;
    }
    const event = JSON.parse(raw.toString("utf8"));
    if (seen.has(event.id)) {
      console.log(`duplicate ${event.id} (${event.type}) ignored`);
    } else {
      seen.add(event.id);
      console.log(`received ${event.id} (${event.type}):`, JSON.stringify(event.data));
    }
    res.writeHead(204).end();
  });
}).listen(port, host, () => console.log(`listening on ${host}:${port}`));

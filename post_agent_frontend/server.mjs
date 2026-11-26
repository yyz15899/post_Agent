import http from "node:http";
import https from "node:https";
import { readFileSync } from "node:fs";
import { parseArgs } from "node:util";

const page = readFileSync(new URL("./public/index.html", import.meta.url));
const backend = new URL(process.env.BACKEND_URL || "http://127.0.0.1:8081");
if (!["http:", "https:"].includes(backend.protocol) || backend.username ||
    backend.password || backend.pathname !== "/" || backend.search || backend.hash) {
  throw new Error("BACKEND_URL must be an HTTP(S) origin such as http://127.0.0.1:8081");
}
const { values } = parseArgs({ options: {
  host: { type: "string" }, port: { type: "string" }, strictPort: { type: "boolean" }
} });
const host = values.host || process.env.FRONTEND_HOST || "127.0.0.1";
const port = Number(values.port || process.env.FRONTEND_PORT || 3000);
if (!Number.isInteger(port) || port < 1 || port > 65535) {
  throw new Error("FRONTEND_PORT must be an integer from 1 to 65535");
}

const hopHeaders = [
  "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
  "te", "trailer", "transfer-encoding", "upgrade"
];
function proxyHeaders(incoming) {
  const headers = { ...incoming };
  for (const key of String(headers.connection || "").split(",")) delete headers[key.trim().toLowerCase()];
  for (const key of hopHeaders) delete headers[key];
  return headers;
}

const server = http.createServer((req, res) => {
  const path = req.url || "/";
  if (path.startsWith("/api/")) {
    const headers = proxyHeaders(req.headers);
    delete headers.host;
    const transport = backend.protocol === "https:" ? https : http;
    const upstream = transport.request(backend, { method: req.method, path, headers }, (response) => {
      const replyHeaders = proxyHeaders(response.headers);
      if (String(replyHeaders["content-type"] || "").startsWith("text/event-stream")) {
        replyHeaders["x-accel-buffering"] = "no";
      }
      res.writeHead(response.statusCode, replyHeaders);
      res.flushHeaders();
      response.on("error", () => res.destroy());
      response.pipe(res);
    });
    upstream.setTimeout(45_000, () => upstream.destroy(new Error("Gateway timeout")));
    upstream.on("error", () => {
      if (res.destroyed) return;
      if (res.headersSent) {
        res.destroy();
        return;
      }
      res.writeHead(502, { "Content-Type": "application/json; charset=utf-8", "Cache-Control": "no-store" });
      res.end(JSON.stringify({ msg: "暂时无法访问 Go 网关，请检查服务是否启动。" }));
    });
    res.on("close", () => {
      if (!res.writableEnded) upstream.destroy();
    });
    req.on("error", () => upstream.destroy());
    req.pipe(upstream);
    return;
  }
  if (["GET", "HEAD"].includes(req.method) && ["/", "/index.html"].includes(path.split("?")[0])) {
    res.writeHead(200, {
      "Content-Type": "text/html; charset=utf-8",
      "Content-Length": page.length,
      "Cache-Control": "no-cache",
      "X-Content-Type-Options": "nosniff"
    });
    res.end(req.method === "HEAD" ? undefined : page);
    return;
  }
  res.writeHead(404, { "Content-Type": "text/plain; charset=utf-8" });
  res.end("Not found");
});

server.listen(port, host, () => {
  console.log("Post Agent frontend: http://" + host + ":" + port);
});

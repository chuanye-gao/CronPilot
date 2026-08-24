const maxRequestBytes = 1024 * 1024;
const upstreamTimeoutMs = 120_000;

export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (request.method === "GET" && url.pathname === "/health") {
      return json({ status: "ok", service: "cronpilot-relay" });
    }
    if (request.method !== "POST") {
      return json({ error: "not found" }, 404);
    }
    if (!await authorized(request, env.CRONPILOT_RELAY_KEY)) {
      return json({ error: "unauthorized" }, 401);
    }

    try {
      const body = await readJSON(request);
      switch (url.pathname) {
        case "/v1/tavily/search":
          return forwardTavily("search", body, env.TAVILY_API_KEY);
        case "/v1/tavily/extract":
          return forwardTavily("extract", body, env.TAVILY_API_KEY);
        case "/v1/gemini/openai/chat/completions":
          return forwardGemini(body, env.GEMINI_API_KEY);
        default:
          return json({ error: "not found" }, 404);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : "invalid request";
      const status = message === "request body is too large" ? 413 : 400;
      return json({ error: message }, status);
    }
  },
};

async function authorized(request, expected) {
  if (!expected) return false;
  const header = request.headers.get("Authorization") || "";
  const supplied = header.startsWith("Bearer ") ? header.slice(7) : "";
  if (!supplied) return false;
  const encoder = new TextEncoder();
  const [left, right] = await Promise.all([
    crypto.subtle.digest("SHA-256", encoder.encode(supplied)),
    crypto.subtle.digest("SHA-256", encoder.encode(expected)),
  ]);
  const a = new Uint8Array(left);
  const b = new Uint8Array(right);
  let different = a.length ^ b.length;
  for (let index = 0; index < Math.min(a.length, b.length); index++) different |= a[index] ^ b[index];
  return different === 0;
}

async function readJSON(request) {
  const declared = Number(request.headers.get("Content-Length") || "0");
  if (declared > maxRequestBytes) throw new Error("request body is too large");
  const text = await request.text();
  if (new TextEncoder().encode(text).byteLength > maxRequestBytes) throw new Error("request body is too large");
  let value;
  try {
    value = JSON.parse(text);
  } catch {
    throw new Error("request body must be valid JSON");
  }
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("request body must be a JSON object");
  return value;
}

async function forwardTavily(operation, body, apiKey) {
  if (!apiKey) return json({ error: "Tavily is not configured" }, 503);
  const payload = { ...body, api_key: apiKey };
  return forward(`https://api.tavily.com/${operation}`, payload, { Authorization: `Bearer ${apiKey}` });
}

async function forwardGemini(body, apiKey) {
  if (!apiKey) return json({ error: "Gemini is not configured" }, 503);
  return forward("https://generativelanguage.googleapis.com/v1beta/openai/chat/completions", body, {
    Authorization: `Bearer ${apiKey}`,
  });
}

async function forward(target, body, extraHeaders) {
  let response;
  try {
    response = await fetch(target, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json", ...extraHeaders },
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(upstreamTimeoutMs),
    });
  } catch (error) {
    const timedOut = error instanceof Error && (error.name === "TimeoutError" || error.name === "AbortError");
    return json({ error: timedOut ? "upstream request timed out" : "upstream request failed" }, timedOut ? 504 : 502);
  }
  const headers = new Headers({ "Content-Type": response.headers.get("Content-Type") || "application/json" });
  const requestID = response.headers.get("x-request-id");
  if (requestID) headers.set("x-upstream-request-id", requestID);
  return new Response(response.body, { status: response.status, headers });
}

function json(value, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json; charset=utf-8", "Cache-Control": "no-store" },
  });
}

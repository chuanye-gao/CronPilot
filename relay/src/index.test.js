import assert from "node:assert/strict";
import test from "node:test";
import worker from "./index.js";

const env = {
  CRONPILOT_RELAY_KEY: "relay-secret",
  TAVILY_API_KEY: "tavily-secret",
  GEMINI_API_KEY: "gemini-secret",
};

test("health endpoint is public", async () => {
  const response = await worker.fetch(new Request("https://relay.example/health"), env);
  assert.equal(response.status, 200);
  assert.deepEqual(await response.json(), { status: "ok", service: "cronpilot-relay" });
});

test("provider routes require the relay key", async () => {
  const response = await worker.fetch(new Request("https://relay.example/v1/tavily/search", {
    method: "POST",
    body: "{}",
  }), env);
  assert.equal(response.status, 401);
});

test("Tavily route injects the provider secret and never forwards the relay key", async (t) => {
  const originalFetch = globalThis.fetch;
  t.after(() => { globalThis.fetch = originalFetch; });
  globalThis.fetch = async (url, init) => {
    assert.equal(url, "https://api.tavily.com/search");
    assert.equal(init.headers.Authorization, "Bearer tavily-secret");
    assert.deepEqual(JSON.parse(init.body), { query: "news", api_key: "tavily-secret" });
    return Response.json({ results: [] });
  };

  const response = await worker.fetch(new Request("https://relay.example/v1/tavily/search", {
    method: "POST",
    headers: { Authorization: "Bearer relay-secret", "Content-Type": "application/json" },
    body: JSON.stringify({ query: "news", api_key: "relay-secret" }),
  }), env);
  assert.equal(response.status, 200);
  assert.deepEqual(await response.json(), { results: [] });
});

test("unknown authenticated route is rejected", async () => {
  const response = await worker.fetch(new Request("https://relay.example/v1/unknown", {
    method: "POST",
    headers: { Authorization: "Bearer relay-secret" },
    body: "{}",
  }), env);
  assert.equal(response.status, 404);
});

# CronPilot Relay

This Cloudflare Worker is a narrow, authenticated egress gateway for CronPilot. It only forwards Tavily Search, Tavily Extract, and Gemini OpenAI-compatible chat requests. It is not a general-purpose HTTP proxy.

## Git deployment from the CronPilot monorepo

Connect the existing `chuanye-gao/CronPilot` repository in Cloudflare Workers Builds. Use `relay` as the root directory, `main` as the production branch, and `pnpm run deploy` as the deploy command. Leave the build command empty and set the build watch include path to `relay/*`. The Cloudflare Worker name must match `cronpilot-relay` from `wrangler.jsonc`.

Add `CRONPILOT_RELAY_KEY`, `TAVILY_API_KEY`, and `GEMINI_API_KEY` under the Worker's runtime **Variables & Secrets**, not as build-only variables.

## Deploy

```bash
cd relay
npm install
npx wrangler login
npx wrangler secret put CRONPILOT_RELAY_KEY
npx wrangler secret put TAVILY_API_KEY
npx wrangler secret put GEMINI_API_KEY
npm run deploy
```

Generate `CRONPILOT_RELAY_KEY` as an independent random value of at least 32 bytes. Do not reuse a provider API key.

After deployment, use the `workers.dev` URL for the first connectivity test. For production, add a Custom Domain such as `relay.example.com` under **Workers & Pages → cronpilot-relay → Settings → Domains & Routes**.

Configure the Weixin Cloud CronPilot service with only:

```text
CRONPILOT_RELAY_URL=https://relay.example.com
CRONPILOT_RELAY_KEY=<same independent random value>
```

Keep `TAVILY_API_KEY` and `GEMINI_API_KEY` only in Cloudflare Secrets. `/health` is public and reveals no secret or provider status. All forwarding routes require the relay key.

# Cloudflare installer endpoint

Deploy the worker after authenticating Wrangler:

```bash
npx wrangler login
npx wrangler deploy --config deploy/cloudflare/wrangler.toml
```

The route serves a self-contained Go installer at `https://toolnet.tech/install`.

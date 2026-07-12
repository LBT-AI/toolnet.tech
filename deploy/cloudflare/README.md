# Cloudflare installer endpoint

Deploy the worker after authenticating Wrangler:

```bash
npx wrangler login
npx wrangler deploy --config deploy/cloudflare/wrangler.toml
```

The route securely proxies the version-controlled `install.sh` at
`https://toolnet.tech/install` and caches it for five minutes.

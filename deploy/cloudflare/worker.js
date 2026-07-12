const INSTALLER_URL = "https://raw.githubusercontent.com/LBT-AI/toolnet.tech/master/install.sh";

export default {
  async fetch(request) {
    const url = new URL(request.url);
    if (url.pathname !== "/install") {
      return new Response("Not found\n", { status: 404 });
    }
    if (request.method !== "GET" && request.method !== "HEAD") {
      return new Response("Method not allowed\n", {
        status: 405,
        headers: { Allow: "GET, HEAD" },
      });
    }

    const upstream = await fetch(INSTALLER_URL, {
      cf: { cacheEverything: true, cacheTtl: 300 },
    });
    if (!upstream.ok) {
      return new Response("Installer is temporarily unavailable\n", { status: 502 });
    }
    return new Response(request.method === "HEAD" ? null : upstream.body, {
      status: 200,
      headers: {
        "Content-Type": "text/x-shellscript; charset=utf-8",
        "Cache-Control": "public, max-age=300",
        "X-Content-Type-Options": "nosniff",
      },
    });
  },
};

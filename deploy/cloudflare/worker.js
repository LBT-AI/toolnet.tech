const INSTALLER = `#!/usr/bin/env bash
set -euo pipefail

BINARY="toolnet"
MODULE="github.com/LBT-AI/toolnet.tech/cmd/toolnet@latest"
INSTALL_DIR="\${INSTALL_DIR:-}"

if ! command -v go >/dev/null 2>&1; then
  echo "error: Go 1.22+ is required (https://go.dev/dl/)" >&2
  exit 1
fi

if [[ -z "\$INSTALL_DIR" ]]; then
  if [[ -w /usr/local/bin ]]; then
    INSTALL_DIR="/usr/local/bin"
  else
    INSTALL_DIR="\$HOME/.local/bin"
  fi
fi

mkdir -p "\$INSTALL_DIR"
echo "Installing TOOLNET CLI..."
GOBIN="\$INSTALL_DIR" go install "\$MODULE"
echo "toolnet installed to \$INSTALL_DIR/\$BINARY"
if [[ ":\$PATH:" != *":\$INSTALL_DIR:"* ]]; then
  echo "Add \$INSTALL_DIR to your PATH."
fi
`;

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

    return new Response(request.method === "HEAD" ? null : INSTALLER, {
      status: 200,
      headers: {
        "Content-Type": "text/x-shellscript; charset=utf-8",
        "Cache-Control": "public, max-age=3600",
        "X-Content-Type-Options": "nosniff",
      },
    });
  },
};

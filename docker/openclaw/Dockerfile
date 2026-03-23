# Custom OpenClaw image with pre-installed tools
# Based on 1panel/openclaw which is already stable in production

FROM 1panel/openclaw:latest

USER root

# Install uv, Python 3.12, and clawhub in a single layer
ENV UV_PYTHON_INSTALL_DIR=/usr/local/share/uv/python
RUN curl -LsSf https://astral.sh/uv/install.sh | env UV_INSTALL_DIR=/usr/local/bin sh && \
    uv python install 3.12 && \
    ln -sf /usr/bin/python3 /usr/local/bin/python && \
    npm install -g clawhub

# Wrapper script: if user has upgraded openclaw via "npm i -g openclaw",
# use the PVC-installed openclaw.mjs directly (same as built-in execution method).
# Always runs via "node <path>/openclaw.mjs" to avoid npm bin wrapper's systemd check.
RUN printf '#!/bin/sh\n\
PVC_MJS="$HOME/.openclaw/npm/lib/node_modules/openclaw/openclaw.mjs"\n\
if [ -f "$PVC_MJS" ]; then\n\
  exec node "$PVC_MJS" "$@"\n\
fi\n\
exec node /app/openclaw.mjs "$@"\n' > /usr/local/bin/openclaw && chmod +x /usr/local/bin/openclaw

USER node

# Pre-install WeChat channel plugin into a staging dir.
# At runtime ~/.openclaw is PVC-mounted, so we install to the default path first
# then move to /opt/openclaw-plugins/ which won't be shadowed by the PVC mount.
RUN node /app/openclaw.mjs plugins install "@tencent-weixin/openclaw-weixin" && \
    mkdir -p /opt/openclaw-plugins && \
    mv /home/node/.openclaw/extensions/openclaw-weixin /opt/openclaw-plugins/ && \
    rm -rf /home/node/.openclaw

# PVC-backed working directory for skills, plugins, and user-installed packages
ENV CLAWHUB_WORKDIR="/home/node/.openclaw"
ENV NPM_CONFIG_PREFIX="/home/node/.openclaw/npm"

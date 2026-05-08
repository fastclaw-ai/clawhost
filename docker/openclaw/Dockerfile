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

# Prepare staging dir for pre-installed plugins (needs root for /opt)
RUN mkdir -p /opt/openclaw-init && chown node:node /opt/openclaw-init

USER node

# Pre-install WeChat channel plugin, then snapshot the entire ~/.openclaw
# tree to /opt/openclaw-init/. The PVC mount on ~/.openclaw at runtime
# would otherwise shadow this; the startup script restores the snapshot
# on first boot.
RUN openclaw plugins install "@tencent-weixin/openclaw-weixin" && \
    cp -a /home/node/.openclaw/. /opt/openclaw-init/ && \
    rm -rf /home/node/.openclaw

# PVC-backed working directory for skills, plugins, and user-installed packages
ENV CLAWHUB_WORKDIR="/home/node/.openclaw"
ENV NPM_CONFIG_PREFIX="/home/node/.openclaw/npm"

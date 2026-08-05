FROM mcr.microsoft.com/devcontainers/javascript-node:26

# Seed ownership of the persistent Claude config volume (mounted at this path in
# devcontainer.json). A fresh named volume inherits the image directory's
# ownership on first use, so this makes it writable by the non-root `node` user.
RUN mkdir -p /home/node/.claude && chown node:node /home/node/.claude

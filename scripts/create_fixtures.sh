#!/bin/bash
# Creates all synthetic test fixtures for Shipcheck's test suite.
set -e
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
F="$(cd "$SCRIPT_DIR/../testdata/fixtures" 2>/dev/null || true; pwd)"
if [ -z "$F" ] || [ ! -d "$F" ]; then
  F="$SCRIPT_DIR/../testdata/fixtures"
fi
rm -rf "$F"
mkdir -p "$F"/{healthy-node,broken-node,python-app,compose-stack,monorepo/apps/api,monorepo/apps/web,security-traps/deep,empty}
touch "$F/empty/.gitkeep"

# ---------- healthy-node: expected to produce ZERO findings ----------
cat > "$F/healthy-node/package.json" <<'EOF'
{
  "name": "healthy-api",
  "version": "1.0.0",
  "private": true,
  "main": "server.js",
  "scripts": {
    "start": "node server.js",
    "build": "esbuild server.js --bundle --outfile=dist/server.js",
    "test": "node --test"
  },
  "dependencies": {
    "express": "^4.18.2"
  },
  "devDependencies": {
    "esbuild": "^0.20.0"
  }
}
EOF
printf 'lockfileVersion: 3\n' > "$F/healthy-node/package-lock.json"
cat > "$F/healthy-node/server.js" <<'EOF'
const express = require("express");
const PORT = process.env.PORT || 3000;
const API_URL = process.env.API_URL || "http://localhost:9000";
const app = express();
app.get("/healthz", (req, res) => res.json({ ok: true }));
const server = app.listen(PORT, () => console.log("listening on " + PORT));
process.on("SIGTERM", () => {
  server.close(() => process.exit(0));
});
EOF
cat > "$F/healthy-node/Dockerfile" <<'EOF'
FROM node:20.11.0-alpine3.19
WORKDIR /app
COPY package.json package-lock.json ./
COPY server.js ./
RUN npm ci --omit=dev
USER node
EXPOSE 3000
HEALTHCHECK --interval=30s CMD wget -qO- http://127.0.0.1:3000/healthz || exit 1
CMD ["node", "server.js"]
EOF
cat > "$F/healthy-node/.dockerignore" <<'EOF'
node_modules
dist
EOF
cat > "$F/healthy-node/.env.example" <<'EOF'
PORT=3000
API_URL=http://localhost:9000
EOF
cat > "$F/healthy-node/docker-compose.yml" <<'EOF'
services:
  web:
    build:
      context: .
      dockerfile: Dockerfile
    ports:
      - "${WEB_PORT:-8080}:3000"
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:3000/healthz"]
      interval: 30s
    environment:
      PORT: 3000
      API_URL: http://api:9000
volumes: {}
EOF
cat > "$F/healthy-node/.gitignore" <<'EOF'
node_modules
.env
EOF

# ---------- broken-node: triggers most rules ----------
cat > "$F/broken-node/package.json" <<'EOF'
{
  "name": "broken-api",
  "version": "0.1.0",
  "scripts": {
    "start": "nodemon server.js",
    "migrate": "node scripts/migrate.js"
  },
  "dependencies": {
    "left-pad": "*",
    "sqlite3": "^5.1.6"
  }
}
EOF
cat > "$F/broken-node/server.js" <<'EOF'
const API_KEY = process.env.API_KEY;
const DATABASE_URL = process.env.DATABASE_URL;
const AWS_KEY = "AKIAIOSFODNN7EXAMPLE";
const password = "hunter2hunter2hunter2";
console.log(API_KEY, DATABASE_URL, AWS_KEY, password);
EOF
cat > "$F/broken-node/.env" <<'EOF'
API_KEY=real-secret-value-1234
EOF
cat > "$F/broken-node/Dockerfile" <<'EOF'
FROM node:latest
WORKDIR /app
COPY package.json ./
COPY config/production.json ./config/
RUN curl -s https://example-installer.sh/install.sh | sh
EXPOSE 3000
CMD npm start
EOF
cat > "$F/broken-node/docker-compose.yml" <<'EOF'
services:
  web:
    build:
      context: .
      dockerfile: Dockerfile
    ports:
      - "8080:4000"
    env_file:
      - .env.missing
    depends_on:
      - db2
    volumes:
      - appdata:/data
      - /var/run/docker.sock:/var/run/docker.sock
    privileged: true
  worker:
    image: busybox
    network_mode: host
    command: sh -c "while true; do sleep 1; done"
EOF
mkdir -p "$F/broken-node/scripts"
cat > "$F/broken-node/scripts/.gitkeep" <<'EOF'
EOF

# ---------- python-app ----------
cat > "$F/python-app/requirements.txt" <<'EOF'
flask
gunicorn==21.2.0
EOF
cat > "$F/python-app/app.py" <<'EOF'
import os
from flask import Flask

app = Flask(__name__)

@app.route("/health")
def health():
    return "ok"

if __name__ == "__main__":
    port = int(os.environ.get("PORT", "8000"))
    app.run(port=port)
EOF
cat > "$F/python-app/Dockerfile" <<'EOF'
FROM python:3.12-slim
WORKDIR /app
COPY requirements.txt ./
COPY app.py ./
RUN pip install --no-cache-dir -r requirements.txt
EXPOSE 8000
HEALTHCHECK CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8000/health')" || exit 1
CMD ["gunicorn", "-b", "0.0.0.0:8000", "app:app"]
EOF
cat > "$F/python-app/.env.example" <<'EOF'
PORT=8000
SECRET_TOKEN=replace-me
EOF

# ---------- compose-stack: clean compose-only project ----------
cat > "$F/compose-stack/docker-compose.yml" <<'EOF'
services:
  web:
    image: nginx:1.25.3
    ports:
      - "${WEB_PORT:-80}:80"
    env_file:
      - .env.web
    depends_on:
      - api
    volumes:
      - assets:/usr/share/nginx/html
    healthcheck:
      test: ["CMD", "true"]
      interval: 30s
  api:
    image: ghcr.io/example/api:1.4.2
    healthcheck:
      test: ["CMD", "true"]
      interval: 30s
volumes:
  assets:
EOF
cat > "$F/compose-stack/.env.web" <<'EOF'
WEB_PORT=8080
EOF

# ---------- monorepo ----------
cat > "$F/monorepo/apps/api/package.json" <<'EOF'
{
  "name": "api",
  "scripts": { "start": "node src/index.js" },
  "dependencies": { "fastify": "^4.24.0" }
}
EOF
mkdir -p "$F/monorepo/apps/api/src"
cat > "$F/monorepo/apps/api/src/index.js" <<'EOF'
const PORT = process.env.API_PORT || 3000;
require("fastify")().listen(PORT);
EOF
cat > "$F/monorepo/apps/web/package.json" <<'EOF'
{
  "name": "web",
  "scripts": { "start": "node server.js", "build": "webpack --mode production" },
  "dependencies": { "react": "^18.2.0" }
}
EOF
cat > "$F/monorepo/apps/web/server.js" <<'EOF'
console.log(process.env.WEB_PORT || 5173);
EOF

# ---------- security-traps ----------
mkdir -p "$F/security-traps/config"
cat > "$F/security-traps/package.json" <<'EOF'
{
  "name": "traps",
  "scripts": {
    "prepare": "touch canary-lifecycle.txt && touch /tmp/shipcheck-canary-tmp",
    "start": "node server.js"
  },
  "dependencies": {}
}
EOF
cat > "$F/security-traps/server.js" <<'EOF'
console.log("trap");
EOF
printf '{"broken": tru\n' > "$F/security-traps/broken.json"
printf 'services:\n  web:\n    image: [unclosed\n    deps: {\n' > "$F/security-traps/broken-compose.yml"
printf 'services:\n  web:\n    build:\n      context: ./no-such-dir\n' > "$F/security-traps/ctx-compose.yml"
# deep nesting
P="$F/security-traps/deep"
for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24 25 26 27 28 29 30; do P="$P/lvl$i"; done
mkdir -p "$P"
echo "bottom" > "$P/leaf.txt"
# oversized file
head -c 6000000 /dev/zero | tr '\0' 'a' > "$F/security-traps/huge.txt"
# binary file
printf 'PNG\x00\x00\x01\x02\x03binary' > "$F/security-traps/image.bin"
# malformed .env.example with weird lines
printf 'VALID_ONE=\nnot a pair at all\nANOTHER_ONE=x\n' > "$F/security-traps/.env.example"

echo "fixtures created"

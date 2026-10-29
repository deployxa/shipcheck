FROM node:20.11.0-alpine3.19
WORKDIR /app
COPY package.json package-lock.json ./
COPY server.js ./
RUN npm ci --omit=dev
USER node
EXPOSE 3000
HEALTHCHECK --interval=30s CMD wget -qO- http://127.0.0.1:3000/healthz || exit 1
CMD ["node", "server.js"]

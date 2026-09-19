FROM node:24-bookworm-slim AS build
RUN npm install --global pnpm@10.11.0
WORKDIR /src
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY apps/web/ apps/web/
COPY apps/admin/package.json apps/admin/package.json
COPY packages/ packages/
COPY tsconfig.base.json ./
RUN pnpm install --frozen-lockfile
RUN pnpm --filter @tap4furry/web build
RUN pnpm --filter @tap4furry/web deploy --legacy --prod /out

FROM node:24-bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends tini && rm -rf /var/lib/apt/lists/*
WORKDIR /app
ENV NODE_ENV=production HOST=0.0.0.0 PORT=4321
COPY --from=build --chown=node:node /out/dist/ ./dist/
COPY --from=build --chown=node:node /out/node_modules/ ./node_modules/
COPY --from=build --chown=node:node /out/package.json ./package.json
USER node
EXPOSE 4321
ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["node", "dist/server/entry.mjs"]

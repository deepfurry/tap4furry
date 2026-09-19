FROM node:24-bookworm-slim AS build
RUN npm install --global pnpm@10.11.0
WORKDIR /src
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
COPY apps/admin/ apps/admin/
COPY apps/web/package.json apps/web/package.json
COPY packages/ packages/
COPY tsconfig.base.json ./
RUN pnpm install --frozen-lockfile
RUN pnpm --filter @tap4furry/admin build

FROM nginxinc/nginx-unprivileged:1.29-alpine
COPY deploy/docker/admin.nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/apps/admin/dist/ /usr/share/nginx/html/
EXPOSE 8080

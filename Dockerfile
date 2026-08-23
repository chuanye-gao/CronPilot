FROM node:22-alpine AS web-build
WORKDIR /src/web

ARG NPM_REGISTRY=https://registry.npmmirror.com
RUN npm install --global pnpm@9.15.5 --registry="${NPM_REGISTRY}" \
    && pnpm config set registry "${NPM_REGISTRY}"

COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY web/ ./
RUN pnpm run build

FROM golang:1.24-alpine AS build
WORKDIR /src

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web-build /src/internal/api/static ./internal/api/static
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/cronpilot ./cmd/cronpilot

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S cronpilot \
    && adduser -S -G cronpilot cronpilot \
    && mkdir -p /data \
    && chown cronpilot:cronpilot /data

COPY --from=build /out/cronpilot /usr/local/bin/cronpilot
COPY cronpilot.docker.yaml /etc/cronpilot/cronpilot.yaml

USER cronpilot:cronpilot
VOLUME ["/data"]
EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/health/live || exit 1

ENTRYPOINT ["/usr/local/bin/cronpilot"]
CMD ["-config", "/etc/cronpilot/cronpilot.yaml"]

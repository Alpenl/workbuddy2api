# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM node:22.22.2-alpine3.23@sha256:8ea2348b068a9544dae7317b4f3aafcdc032df1647bb7d768a05a5cad1a7683f AS web-deps
ENV PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH
WORKDIR /src/frontend
RUN corepack enable && corepack prepare pnpm@9.15.9 --activate
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN --mount=type=cache,id=pnpm-store,target=/pnpm/store \
    pnpm config set store-dir /pnpm/store && \
    pnpm install --frozen-lockfile

FROM web-deps AS web-build
COPY frontend/ ./
RUN pnpm test && pnpm build

FROM --platform=$BUILDPLATFORM golang:1.26.2-alpine3.23@sha256:f85330846cde1e57ca9ec309382da3b8e6ae3ab943d2739500e08c86393a21b1 AS go-build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY --from=web-build /src/internal/web/dist ./internal/web/dist
RUN --mount=type=cache,target=/root/.cache/go-build go test ./...
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -buildvcs=false -ldflags="-s -w" -o /out/wb2api ./cmd/server

FROM alpine:3.23.3@sha256:25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659 AS runtime
RUN apk add --no-cache ca-certificates tzdata wget && \
    addgroup -S -g 1000 app && \
    adduser -S -D -H -h /app -u 1000 -G app app && \
    mkdir -p /app/auths /app/data && \
    chown -R app:app /app

WORKDIR /app
COPY --from=go-build --chown=app:app /out/wb2api /app/wb2api
COPY --chown=app:app config.example.json /app/config.json

ENV TZ=Asia/Shanghai
USER 1000:1000
EXPOSE 7863
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:7863/healthz || exit 1

ENTRYPOINT ["/app/wb2api"]
CMD ["-config", "/app/config.json"]

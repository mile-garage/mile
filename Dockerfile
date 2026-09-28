# syntax=docker/dockerfile:1

# 1) Frontend (built once, on the build machine's architecture)
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# 2) Go binary with the frontend embedded, cross-compiled for the target platform
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web/embed.go ./web/embed.go
COPY --from=web /web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /out/mile ./cmd/mile \
 && mkdir -p /out/data

# 3) Final image: the binary only
FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.title="MILE" \
      org.opencontainers.image.description="Self-hosted vehicle deadlines, expenses and fuel tracker" \
      org.opencontainers.image.source="https://github.com/mile-garage/mile" \
      org.opencontainers.image.licenses="AGPL-3.0-only" \
      org.opencontainers.image.vendor="Gabriele Menghi"
COPY --from=build /out/mile /mile
COPY --from=build --chown=65532:65532 /out/data /data
ENV MILE_DATA_DIR=/data \
    MILE_ADDR=:8080 \
    TZ=Europe/Rome
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s CMD ["/mile", "healthcheck"]
ENTRYPOINT ["/mile"]

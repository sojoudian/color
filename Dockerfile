# syntax=docker/dockerfile:1

# Build stage runs on the builder's native platform and cross-compiles for
# the target, so multi-arch builds never need QEMU emulation.
# Digest-pinned (Dependabot keeps it fresh); tag kept for humans.
FROM --platform=$BUILDPLATFORM golang:1.26.4-alpine@sha256:f23e8b227fb4493eabe03bede4d5a32d04092da71962f1fb79b5f7d1e6c2a17f AS build
WORKDIR /src

COPY go.* ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY cmd/ cmd/
COPY internal/ internal/

ARG TARGETOS TARGETARCH
# .dockerignore excludes .git, so VCS stamping is unavailable; CI passes the
# version explicitly instead.
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/color ./cmd/color

# distroless static: ~2 MiB, no shell, ca-certificates + tzdata included.
# Root variant: the server binds port 80 for drop-in compatibility with
# jpetazzo/color. debian13 — the debian12 line is EOL September 2026.
FROM gcr.io/distroless/static-debian13@sha256:3592aa8171c77482f62bbc4164e6a2d141c6122554ace66e5cc910cadb961ff0

LABEL org.opencontainers.image.title="color" \
      org.opencontainers.image.description="Tiny web server for colorful Kubernetes demos: page color comes from the Deployment name" \
      org.opencontainers.image.source="https://github.com/sojoudian/color" \
      org.opencontainers.image.licenses="MIT"

COPY --from=build /out/color /color

ENV PORT=80
EXPOSE 80

# Exec-form healthcheck for plain `docker run` users; Kubernetes ignores
# this and uses the /healthz and /readyz probes instead.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/color", "-healthcheck"]

ENTRYPOINT ["/color"]

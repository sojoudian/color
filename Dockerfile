# syntax=docker/dockerfile:1

# Build stage runs on the builder's native platform and cross-compiles for
# the target, so multi-arch builds never need QEMU emulation.
# Digest-pinned (Dependabot keeps it fresh); tag kept for humans.
FROM --platform=$BUILDPLATFORM golang:1.26.4-alpine@sha256:bd14630652464086289693533d25b791aa9ae7481e784d7eac5d4c948e9736ea AS build
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

# distroless static: ~2 MiB, no shell, ca-certificates + tzdata + nonroot
# user (65532) included. debian13 — the debian12 line is EOL September 2026.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:963fa6c544fe5ce420f1f54fb88b6fb01479f054c8056d0f74cc2c6000df5240

LABEL org.opencontainers.image.title="color" \
      org.opencontainers.image.description="Tiny web server for colorful Kubernetes demos: page color comes from the Deployment name" \
      org.opencontainers.image.source="https://github.com/sojoudian/color" \
      org.opencontainers.image.licenses="MIT"

COPY --from=build /out/color /color

ENV PORT=8080
EXPOSE 8080
USER nonroot:nonroot

# Exec-form healthcheck for plain `docker run` users; Kubernetes ignores
# this and uses the /healthz and /readyz probes instead.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/color", "-healthcheck"]

ENTRYPOINT ["/color"]

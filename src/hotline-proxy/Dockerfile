# syntax=docker/dockerfile:1
#
# The hotline-proxy image. Build it from the root of the repo, which holds
# both the proxy and the vendor specs:
#
#   docker buildx build -f src/hotline-proxy/Dockerfile --build-arg VERSION=v0.1.0 .
#
# Decisions (Peter, 2026-11-01): docs/proxy-docker-image.md.

# The build runs on the build machine's own platform and Go cross-compiles
# for the target one, so a multi-arch build needs no emulation.
FROM --platform=$BUILDPLATFORM golang:1.27.3-trixie@sha256:c65ecfe613b539ac74c62a4c3b2c27e8f8a6076384c8f27e8a3c0f3be17cc385 AS build
ARG TARGETOS
ARG TARGETARCH
# VERSION is the release's git tag, printed by hotline-proxy version.
ARG VERSION
WORKDIR /src
COPY src/hotline-proxy/go.mod src/hotline-proxy/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY src/hotline-proxy/ ./
# One static program with Go's own name lookup: no C library needed.
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOWORK=off \
    go build -trimpath -ldflags "-s -w -X main.buildVersion=${VERSION}" -o /out/hotline-proxy ./cmd

# Google's distroless static image: CA bundle, time zone data and the
# nonroot user, with no shell and no package manager.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329
# The stable vendor specs, in their own layer: they change rarely, so a new
# program reuses this layer. A config points to them by absolute path.
COPY specs/ /usr/share/hotline/specs/
COPY --from=build /out/hotline-proxy /hotline-proxy
USER 65532:65532
EXPOSE 8080 8081
ENTRYPOINT ["/hotline-proxy"]
CMD ["run", "--config", "/etc/hotline/config/hotline-proxy.yaml"]

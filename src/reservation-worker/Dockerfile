# Current stable Go release as of writing: 1.26.5 (go.dev/blog/go1.26; go 1.27
# was still an unreleased RC at the time). Digest-pinned for reproducible
# builds, not a floating tag -- re-resolve both tag and digest before
# rebuilding long after this file was written:
#   docker buildx imagetools inspect golang:1.26.5-bookworm
FROM golang:1.26.5-bookworm@sha256:6c5605ab3a9a9fb3c4eafe5b3d63cdbf3881caf113262b67862547b54a9db599 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/reservation-worker .

# static-debian12 (not scratch): pgx dials Postgres over TLS-capable
# connection params depending on DATABASE_URL and needs a CA trust store,
# which scratch doesn't provide. :nonroot bundles a non-root user (65532)
# already.
#
# :nonroot is a floating tag (github.com/GoogleContainerTools/distroless),
# republished periodically with security patches -- digest-pinned here for
# reproducible builds. Re-resolve before rebuilding long after this file was
# written:
#   docker buildx imagetools inspect gcr.io/distroless/static-debian12:nonroot
FROM gcr.io/distroless/static-debian12:nonroot@sha256:f5b485ea962d9bd1186b2f6b3a061191539b905b82ec395de78cbfae51f20e35

COPY --from=builder /out/reservation-worker /reservation-worker

USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/reservation-worker"]

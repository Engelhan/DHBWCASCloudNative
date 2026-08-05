# Current stable Go release as of writing: 1.26.5 (go.dev/blog/go1.26).
# Re-check for a newer patch release before rebuilding long after this file
# was written.
FROM golang:1.26.5-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/notification-worker .

# static-debian12 (not scratch): the optional SMTP and Kafka-mTLS paths both
# do real TLS against external hosts and need a CA trust store, which
# scratch doesn't provide. :nonroot bundles a non-root user (65532) already.
#
# :nonroot is a floating tag (github.com/GoogleContainerTools/distroless) --
# pin to a digest for reproducible builds, e.g.:
#   crane digest gcr.io/distroless/static-debian12:nonroot
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/notification-worker /notification-worker

USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/notification-worker"]

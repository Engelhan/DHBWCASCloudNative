# Digest-pinned; re-resolve before rebuilding much later:
#   docker buildx imagetools inspect golang:1.26.5-bookworm
FROM golang:1.26.5-bookworm@sha256:6c5605ab3a9a9fb3c4eafe5b3d63cdbf3881caf113262b67862547b54a9db599 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/catalog-api .

# Digest-pinned:
#   docker buildx imagetools inspect gcr.io/distroless/static-debian12:nonroot
FROM gcr.io/distroless/static-debian12:nonroot@sha256:f5b485ea962d9bd1186b2f6b3a061191539b905b82ec395de78cbfae51f20e35
COPY --from=build /out/catalog-api /catalog-api
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/catalog-api"]

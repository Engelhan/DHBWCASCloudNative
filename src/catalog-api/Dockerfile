FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/catalog-api .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/catalog-api /catalog-api
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/catalog-api"]

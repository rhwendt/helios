FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -o /flow-enricher ./cmd/flow-enricher/

FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.source="https://github.com/helios/helios"
LABEL org.opencontainers.image.description="Helios Flow Enricher - enriches flow data with NetBox metadata and GeoIP"

COPY --from=builder /flow-enricher /flow-enricher
COPY --from=builder /build/data/GeoLite2-City.mmdb /data/GeoLite2-City.mmdb
COPY --from=builder /build/data/GeoLite2-ASN.mmdb /data/GeoLite2-ASN.mmdb

USER nonroot:nonroot

EXPOSE 8080
EXPOSE 9090

ENTRYPOINT ["/flow-enricher"]

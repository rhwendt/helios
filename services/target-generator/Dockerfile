FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -o /target-generator ./cmd/target-generator/

FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.source="https://github.com/helios/helios"
LABEL org.opencontainers.image.description="Helios Target Generator - syncs NetBox inventory to monitoring targets"

COPY --from=builder /target-generator /target-generator

USER nonroot:nonroot

EXPOSE 8080

ENTRYPOINT ["/target-generator"]

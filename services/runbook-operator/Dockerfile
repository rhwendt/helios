FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -o /runbook-operator ./cmd/operator/

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -o /runbook-executor ./cmd/executor/

FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.source="https://github.com/helios/helios"
LABEL org.opencontainers.image.description="Helios Runbook Operator - automated network remediation"

COPY --from=builder /runbook-operator /runbook-operator
COPY --from=builder /runbook-executor /runbook-executor

USER nonroot:nonroot

EXPOSE 8080
EXPOSE 8081

ENTRYPOINT ["/runbook-operator"]

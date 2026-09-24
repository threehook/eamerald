# Local dev image: builds laadpalen-api straight from source with `go build`, the same pattern as k8s/Dockerfile.dev
# uses for mrldd. Docker Desktop's Kubernetes shares the local Docker daemon, so no registry push is needed:
#
#   docker build -f examples/laadpalen/api/Dockerfile -t laadpalen-api:dev .

# Pinned by digest, not just the "1.27" tag: a floating tag forces a registry manifest check on every build even
# when the image is already cached locally, and that check itself counts against Docker Hub's anonymous pull rate
# limit. An immutable digest reference skips it.
FROM golang:1.27@sha256:f44f6e88636cfb311f9ebace870ded69d943f227bb3cb27d32ffd84ea18c43ea AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/laadpalen-api ./examples/laadpalen/api

# Pinned by digest, not just the "latest" tag - see above.
FROM alpine:latest@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /out/laadpalen-api /app/laadpalen-api

ENTRYPOINT ["./laadpalen-api"]

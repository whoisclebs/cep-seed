# syntax=docker/dockerfile:1
# Dockerfile — cep-seed image.
# Contains cep-updater binary only. No eDNE source or cep.db is embedded.
# This image updates an API-initialized database on a shared volume.

FROM golang:1.26-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /build/cep-updater -trimpath ./cmd/cep-updater/

FROM alpine:3.20

# ca-certificates for HTTPS download of the Correios archive.
RUN apk add --no-cache ca-certificates

# Fixed non-root user and group (UID/GID 1000).
RUN addgroup -g 1000 cep && \
    adduser -D -u 1000 -G cep cep && \
    mkdir -p /data && \
    chown cep:cep /data

COPY --from=builder /build/cep-updater /usr/local/bin/cep-updater

USER cep:cep
WORKDIR /data

# Database path, source URL, download timeout all configurable via env vars.
ENV CEP_DB_PATH=/data/cep.db
ENV CEP_EDNE_URL=https://www2.correios.com.br/sistemas/edne/download/eDNE_Basico.zip

ENTRYPOINT ["cep-updater"]

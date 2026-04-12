# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.26
ARG ALPINE_VERSION=3.22

FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS builder
WORKDIR /src

# git is needed for some module resolution paths in private/mirrored setups.
RUN apk add --no-cache git

# Version build args
ARG VERSION=dev
ARG COMMIT_SHA=none
ARG BUILD_DATE=unknown

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -buildid= -X main.Version=${VERSION} -X main.CommitSHA=${COMMIT_SHA} -X main.BuildDate=${BUILD_DATE}" \
    -o /out/sentinel .

FROM alpine:${ALPINE_VERSION}
WORKDIR /app

# tzdata keeps timestamps/logging consistent; wget is used by docker-compose healthcheck.
RUN apk add --no-cache ca-certificates tzdata wget

COPY --from=builder /out/sentinel /usr/local/bin/sentinel

RUN mkdir -p /app/data

EXPOSE 8080 9090

ENTRYPOINT ["/usr/local/bin/sentinel"]
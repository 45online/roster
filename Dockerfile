# syntax=docker/dockerfile:1.7

# ──── Build stage ─────────────────────────────────────────────────────
FROM golang:1.26-alpine AS build

WORKDIR /src

# Layered copy so go.mod/go.sum changes don't bust the source layer.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${BUILD_DATE}" \
    -o /out/roster \
    ./cmd/roster

# ──── Run stage ───────────────────────────────────────────────────────
FROM alpine:3.20

# ca-certificates: needed for HTTPS to GitHub / Jira / Slack / Claude APIs.
# tzdata: lets the user pick a TZ via env if they care; small.
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 1000 -h /home/roster roster

USER roster
WORKDIR /home/roster

# Mount points the user is expected to bind in. ~/.roster holds creds /
# audit / cursors; /work is where the user mounts a repo for `roster init`
# or `roster takeover` to find .roster/config.yml.
VOLUME ["/home/roster/.roster", "/work"]

COPY --from=build /out/roster /usr/local/bin/roster

ENTRYPOINT ["/usr/local/bin/roster"]
CMD ["--help"]

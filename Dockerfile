FROM node:22-bookworm AS frontend-builder

WORKDIR /build/web_frontend

# Copy frontend package files and install dependencies
COPY web_frontend/package*.json ./
RUN npm ci --ignore-scripts

# Copy frontend source and build
COPY web_frontend/ ./
RUN npm run build

FROM golang:1.24 AS builder

RUN apt-get update && apt-get install -y libolm-dev && apt-get clean

WORKDIR /go/src/github.com/Scrin/siikabot/

# Copy Go module files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy all source code
COPY . ./

# Copy the built frontend into the embed location
COPY --from=frontend-builder /build/web_frontend/dist ./bot/frontend/dist

# Build the Go binary with embedded frontend
RUN CGO_ENABLED=1 go build -tags embed -o /go/bin/siikabot

FROM debian:bookworm-slim

RUN apt-get update && \
  apt-get install -y --no-install-recommends libolm3 ca-certificates inetutils-ping traceroute && \
  apt-get clean && \
  rm -rf /var/lib/apt/lists/*

COPY --from=builder /go/bin/siikabot /usr/local/bin/siikabot

CMD ["siikabot"]

FROM golang:1.24 AS builder

RUN apt-get update && apt-get install -y libolm-dev && apt-get clean

WORKDIR /go/src/github.com/Scrin/siikabot/
COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=1 go build -o /go/bin/siikabot

FROM debian:bookworm-slim

RUN apt-get update && \
  apt-get install -y --no-install-recommends libolm3 ca-certificates inetutils-ping traceroute && \
  apt-get clean && \
  rm -rf /var/lib/apt/lists/*

COPY --from=builder /go/bin/siikabot /usr/local/bin/siikabot

CMD ["siikabot"]

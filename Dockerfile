FROM golang:1.24

RUN apt-get update && apt-get install -y libolm-dev inetutils-ping traceroute && apt-get clean

WORKDIR /go/src/github.com/Scrin/siikabot/
COPY . ./
RUN go install .

CMD siikabot

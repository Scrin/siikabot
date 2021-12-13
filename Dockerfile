FROM golang:1.17

RUN apt-get update && apt-get install -y inetutils-ping traceroute && apt-get clean

WORKDIR /go/src/github.com/Scrin/siikabot/
COPY . ./
RUN go install .

CMD siikabot

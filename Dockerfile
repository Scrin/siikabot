FROM golang:1.14

RUN apt-get update && apt-get install -y traceroute && apt-get clean

COPY . /go/src/siikabot/
RUN go get siikabot/...
RUN go install siikabot

CMD siikabot

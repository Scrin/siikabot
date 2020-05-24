FROM golang:1.14

COPY . /go/src/siikabot/
RUN go get siikabot/...
RUN go install siikabot

CMD siikabot

FROM golang:1.14 as build

COPY . /go/src/siikabot/
RUN go get siikabot/...
RUN go install siikabot

FROM alpine

RUN mkdir /lib64 && ln -s /lib/libc.musl-x86_64.so.1 /lib64/ld-linux-x86-64.so.2
COPY --from=build /go/bin/siikabot /siikabot
CMD /siikabot

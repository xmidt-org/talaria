FROM golang:alpine as builder
MAINTAINER Jack Murdock <jack_murdock@comcast.com>

# build the binary
WORKDIR /go/src
COPY src/ /go/src/

RUN go build -o talaria_linux_amd64 talaria

EXPOSE 6200 6201 6202
RUN mkdir -p /etc/talaria
VOLUME /etc/talaria

# the actual image
FROM alpine:latest
RUN apk --no-cache add ca-certificates
RUN mkdir -p /etc/talaria
VOLUME /etc/talaria
WORKDIR /root/
COPY --from=builder /go/src/talaria_linux_amd64 .
ENTRYPOINT ["./talaria_linux_amd64"]

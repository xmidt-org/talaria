FROM golang:alpine as builder
MAINTAINER Jack Murdock <jack_murdock@comcast.com>

ENV build_gopath /go/src/talaria

# fetch needed software
RUN apk add --update --repository https://dl-3.alpinelinux.org/alpine/edge/testing/ git curl
RUN curl https://glide.sh/get | sh

COPY src/ ${build_gopath}/src

WORKDIR ${build_gopath}/src
ENV GOPATH ${build_gopath} 

# fetch golang dependencies
RUN glide -q install --strip-vendor

# build the binary
WORKDIR ${build_gopath}/src/talaria
RUN go build -o talaria_linux_amd64 talaria

# prep the actual image
FROM alpine:latest
RUN apk --no-cache add ca-certificates
EXPOSE 6100 6101 6102
RUN mkdir -p /etc/talaria
VOLUME /etc/talaria
WORKDIR /root/
COPY --from=builder /go/src/talaria/src/talaria/talaria_linux_amd64 .
ENTRYPOINT ["./talaria_linux_amd64"]

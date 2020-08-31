FROM docker.io/library/golang:1.14-alpine as builder

LABEL MAINTAINER="Jack Murdock <jack_murdock@comcast.com>"

WORKDIR /go/src/github.com/xmidt-org/talaria

ARG VERSION=unknown
ARG GITCOMMIT=unknown
ARG BUILDTIME=unknown

ADD https://github.com/geofffranks/spruce/releases/download/v1.25.2/spruce-linux-amd64 /usr/local/bin/spruce
RUN chmod +x /usr/local/bin/spruce

RUN apk add --no-cache --no-progress \
    ca-certificates \
    make \
    git \
    openssh \
    gcc \
    libc-dev \
    upx

COPY . .
RUN make build

FROM alpine:latest

COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /go/src/github.com/xmidt-org/talaria/talaria.yaml /talaria.yaml
COPY --from=builder /go/src/github.com/xmidt-org/talaria/talaria /talaria
COPY --from=builder /go/src/github.com/xmidt-org/talaria/deploy/Dockerfile /go/src/github.com/xmidt-org/talaria/NOTICE /go/src/github.com/xmidt-org/talaria/LICENSE /go/src/github.com/xmidt-org/talaria/CHANGELOG.md /
COPY --from=builder /go/src/github.com/xmidt-org/talaria/deploy/packaging/entrypoint.sh /entrypoint.sh
COPY --from=builder /go/src/github.com/xmidt-org/talaria/deploy/packaging/talaria_spruce.yaml /tmp/talaria_spruce.yaml
COPY --from=builder /usr/local/bin/spruce /spruce

RUN mkdir /etc/talaria/ && touch /etc/talaria/talaria.yaml && chmod 666 /etc/talaria/talaria.yaml

USER nobody

ENTRYPOINT ["/entrypoint.sh"]

EXPOSE 6200
EXPOSE 6201
EXPOSE 6202
EXPOSE 6203

CMD ["/talaria"]
FROM docker.io/library/golang:1.19-alpine as builder

WORKDIR /src

RUN apk add --no-cache --no-progress \
    ca-certificates \
    curl

# Download spruce here to eliminate the need for curl in the final image
RUN mkdir -p /go/bin && \
    arch=$(arch | sed s/aarch64/arm64/ | sed s/x86_64/amd64/) && \
    curl -L -o /go/bin/spruce https://github.com/geofffranks/spruce/releases/download/v1.30.2/spruce-linux-${arch} && \
    chmod +x /go/bin/spruce && \
    sha1sum /go/bin/spruce

COPY . .

##########################
# Build the final image.
##########################

FROM alpine:latest

# Copy over the standard things you'd expect.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt  /etc/ssl/certs/
COPY talaria /
COPY ./.release/docker/entrypoint.sh  /

# Copy over spruce and the spruce template file used to make the actual configuration file.
COPY .release/docker/talaria_spruce.yaml  /tmp/talaria_spruce.yaml
COPY --from=builder /go/bin/spruce        /bin/

# Include compliance details about the container and what it contains.
COPY Dockerfile /
COPY NOTICE     /
COPY LICENSE    /

# Make the location for the configuration file that will be used.
RUN     mkdir /etc/talaria/ \
    &&  touch /etc/talaria/talaria.yaml \
    &&  chmod 666 /etc/talaria/talaria.yaml

USER nobody

ENTRYPOINT ["/entrypoint.sh"]

EXPOSE 6100
EXPOSE 6101
EXPOSE 6102
EXPOSE 6103

CMD ["/talaria"]

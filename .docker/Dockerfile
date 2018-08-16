FROM centos:6
MAINTAINER Jack Murdock <jack_murdock@comcast.com>

ADD ./talaria talaria
RUN mkdir -p /etc/talaria
VOLUME /etc/talaria

ENTRYPOINT ./talaria

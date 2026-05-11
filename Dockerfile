ARG BASE_IMAGE=alpine:3.20.3
FROM ${BASE_IMAGE}
MAINTAINER yurisa <rain.com>
WORKDIR /pedis
ADD pedis /pedis/bin/
ADD config.json /pedis
RUN mkdir -p /pedis/run && chmod 777 /pedis/run
ENTRYPOINT ["/pedis/bin/pedis"]
EXPOSE 6399/tcp
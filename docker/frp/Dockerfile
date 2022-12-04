FROM alpine:latest

ARG frp_version

RUN apk add --virtual .build-dependencies --no-cache openssl \
  && mkdir -p /etc/frp \
  && cd /tmp \
  && wget -O frp.tar.gz "https://github.com/fatedier/frp/releases/download/v${frp_version}/frp_${frp_version}_linux_amd64.tar.gz" \
  && tar -xzf frp.tar.gz \
  && mv ./frp_${frp_version}_linux_amd64/frps /usr/local/bin \
  && rm -rf /tmp/* \
  && apk del .build-dependencies

WORKDIR /etc/frp

ENTRYPOINT frps $FRP_ARGS

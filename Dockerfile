# Pinned by digest, not just the "latest" tag: a floating tag forces a
# registry manifest check on every build even when the image is already
# cached locally, and that check itself counts against Docker Hub's
# anonymous pull rate limit. An immutable digest reference skips it.
FROM alpine:latest@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH
ARG BUILDPLATFORM

RUN echo "BUILDPLATFORM=$BUILDPLATFORM" \
 && echo "TARGETPLATFORM=$TARGETPLATFORM" \
 && echo "TARGETOS=$TARGETOS" \
 && echo "TARGETARCH=$TARGETARCH"

RUN apk add --no-cache bash tzdata ca-certificates

RUN mkdir /config && \
    mkdir /certs && \
    mkdir /db && \
    mkdir /decisions

VOLUME ["/config", "/certs", "/db", "/decisions"]

ENV EAMERALD_RUNNING_IN_CONTAINER=true

WORKDIR /app

COPY \
${TARGETPLATFORM}/mrldd \
${TARGETPLATFORM}/mrld-db \
${TARGETPLATFORM}/mrld-backup \
/app/

ENTRYPOINT ["./mrldd"]
CMD ["run", "-c", "/config/config.yaml"]

ARG        GOLANG_TAG=latest
ARG        ALPINE_TAG=latest

FROM       golang:${GOLANG_TAG} AS builder

WORKDIR    /app
COPY       . .
RUN        GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go install -ldflags="-w -s"


FROM       alpine:${ALPINE_TAG}

LABEL      org.opencontainers.image.title="MemGator" \
           org.opencontainers.image.description="A Memento Aggregator CLI and Server in Go" \
           org.opencontainers.image.authors="Sawood Alam <@ibnesayeed>" \
           org.opencontainers.image.source="https://github.com/oduwsdl/MemGator" \
           org.opencontainers.image.url="https://hub.docker.com/r/oduwsdl/memgator" \
           org.opencontainers.image.licenses="MIT"

RUN        apk add ca-certificates
COPY       --from=builder /go/bin/memgator /bin/

ENTRYPOINT ["memgator"]

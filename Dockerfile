FROM golang:1.7
MAINTAINER Sawood Alam <ibnesayeed@gmail.com>

COPY . /go/src/github.com/oduwsdl/memgator
WORKDIR /go/src/github.com/oduwsdl/memgator
RUN go install -v

ENTRYPOINT ["memgator"]

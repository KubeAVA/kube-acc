FROM ubuntu:18.04 AS build

ENV GOLANG_VERSION 1.17.1
ENV GOPATH /go
ENV PATH $GOPATH/bin:/usr/local/go/bin:$PATH

WORKDIR /go/src/kube-gpu

COPY . .

RUN apt update && \
    apt install -y g++ wget make && \
    wget -nv -O - https://storage.googleapis.com/golang/go${GOLANG_VERSION}.linux-amd64.tar.gz | tar -C /usr/local -xz && \
    go env -w GOPROXY=https://goproxy.cn,direct && \
    make

FROM alpine:3.9

COPY --from=build /go/src/kube-gpu/bin/kube-gpu /usr/bin/kube-gpu

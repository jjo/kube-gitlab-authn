FROM golang:1.13-alpine as build

ARG SRC_REPO=github.com/jjo/kube-gitlab-authn
ARG SRC_TAG=master
ARG ARCH=amd64
ARG BINARY_BUILD=main
ARG BINARY=kube-gitlab-authn

RUN apk --no-cache --update add ca-certificates make git

#RUN go get ${SRC_REPO}
COPY . /go/src/${SRC_REPO}
WORKDIR ${GOPATH}/src/${SRC_REPO}
RUN make
RUN cp -p _output/${BINARY_BUILD} /${BINARY}

FROM alpine:3.7
RUN apk --no-cache --update add ca-certificates
MAINTAINER JuanJo Ciarlante <juanjosec@gmail.com>

COPY --from=build ${BINARY} /${BINARY}

USER 1001
EXPOSE 3000
CMD ["/kube-gitlab-authn"]

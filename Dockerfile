FROM golang:alpine as build
RUN mkdir -p /usr/src/MegaLAN
WORKDIR /usr/src/MegaLAN
COPY go.mod .
RUN go get github.com/mistsys/tuntap && \
    go get github.com/vishvananda/netlink
COPY *.go ./
RUN go build .
FROM alpine
WORKDIR /usr/bin/
COPY --from=build /usr/src/MegaLAN/megalan .
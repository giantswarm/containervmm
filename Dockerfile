FROM golang:1.16-alpine AS build

ENV GO111MODULE=on

WORKDIR /usr/src/app

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o ./bin/containervmm ./cmd/main.go \
    && chmod +x ./bin/containervmm

FROM fedora:34

RUN dnf -y update \
    && dnf -y install qemu-system-x86 xfsprogs \
    && dnf clean all

COPY --from=build /usr/src/app/bin /usr/local/bin

ENTRYPOINT ["containervmm"]

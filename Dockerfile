FROM golang:1.17
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
ARG COMMITSHA
RUN go build -ldflags "-X main.COMMITSHA=$COMMITSHA" -o cec-core cmd/main.go



FROM ubuntu:focal

WORKDIR /app

RUN apt update && apt install ca-certificates -y

COPY --from=0 /build/cec-core /app/cec-core
CMD ["/app/cec-core"]

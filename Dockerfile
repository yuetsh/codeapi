FROM golang:1.23-alpine AS builder
ARG CGO_ENABLED=0
WORKDIR /app

COPY go.mod go.sum ./
RUN go env -w GO111MODULE=on
RUN go env -w GOPROXY=https://goproxy.cn,direct
RUN go mod download
COPY main.go ./

RUN go build -o api

FROM scratch
COPY --from=builder /app/api /api
ENTRYPOINT ["/api"]
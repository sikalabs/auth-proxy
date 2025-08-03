FROM golang:1.24.4-bullseye as builder
WORKDIR /build
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build

FROM debian:12.11-slim
LABEL org.opencontainers.image.source https://github.com/sikalabs/auth-proxy
COPY \
  --from=builder \
  /build/auth-proxy \
  /usr/local/bin/auth-proxy
CMD ["/usr/local/bin/auth-proxy"]
EXPOSE 8000

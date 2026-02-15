FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git nodejs npm

WORKDIR /src

# Cache Go modules.
COPY go.mod go.sum ./
RUN go mod download

# Build frontend.
COPY frontend/ frontend/
RUN cd frontend && npm ci && npm run build

# Build backend.
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /wgpilot ./cmd/wgpilot

# Runtime image.
FROM alpine:3.20

RUN apk add --no-cache ca-certificates nftables iptables wireguard-tools

COPY --from=builder /wgpilot /usr/local/bin/wgpilot

RUN mkdir -p /var/lib/wgpilot /etc/wgpilot

EXPOSE 8080 443

VOLUME ["/var/lib/wgpilot", "/etc/wgpilot"]

ENTRYPOINT ["wgpilot"]
CMD ["serve"]

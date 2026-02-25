# Build stage
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /pgpulse-server ./cmd/pgpulse-server/

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /pgpulse-server /usr/local/bin/pgpulse-server

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/pgpulse-server"]

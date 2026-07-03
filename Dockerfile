FROM golang:1.20-alpine AS builder

WORKDIR /app
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /pdcarchive ./cmd/pdcarchive

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=builder /pdcarchive /app/pdcarchive

EXPOSE 8080
CMD ["/app/pdcarchive", "serve"]

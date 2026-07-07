FROM golang:1.26-alpine AS builder

WORKDIR /app
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /pdcarchive ./cmd/pdcarchive

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=builder /pdcarchive /app/pdcarchive

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/pdcarchive"]

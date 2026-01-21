FROM golang:1.24-alpine as builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o care-service ./cmd/api/main.go

FROM alpine:latest

# Install netcat for healthchecks
RUN apk add --no-cache netcat-openbsd

WORKDIR /app

COPY --from=builder /app/care-service .
RUN chmod 755 ./care-service

EXPOSE 8080

CMD ["./care-service"]


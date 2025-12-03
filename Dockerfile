FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o main

FROM alpine:latest AS runner

WORKDIR /app
COPY --from=builder /app/main .
EXPOSE 3000

CMD ["./main"]

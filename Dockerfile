FROM golang:1.22 AS builder
WORKDIR /src

COPY go.mod .
COPY go.sum .
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

RUN --mount=type=cache,target=/root/.cache/go-build \
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/app .

FROM gcr.io/distroless/base-debian12 AS runtime
WORKDIR /app
COPY --from=builder /out/app /app/app

ENV PORT=3000
EXPOSE 3000

USER nonroot:nonroot
ENTRYPOINT ["/app/app"]

# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build
WORKDIR /src

RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/app

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=build /out/app /app/app
COPY configs/local.yaml /app/configs/local.yaml

ENV CONFIG_PATH=/app/configs/local.yaml

EXPOSE 8080
USER 65534:65534
ENTRYPOINT ["/app/app"]

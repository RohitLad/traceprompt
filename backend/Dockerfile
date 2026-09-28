# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /app/api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -o /app/worker ./cmd/worker

FROM gcr.io/distroless/static:nonroot AS runner
WORKDIR /app
COPY --from=builder /app/api /app/api
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
USER nonroot:nonroot
EXPOSE 3000
ENTRYPOINT ["/app/api"]

FROM gcr.io/distroless/static:nonroot AS worker
WORKDIR /app
COPY --from=builder /app/worker /app/worker
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
USER nonroot:nonroot
ENTRYPOINT ["/app/worker"]

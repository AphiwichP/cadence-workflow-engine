FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o cadence-worker .

FROM gcr.io/distroless/static-debian12
COPY --from=builder /app/cadence-worker /cadence-worker
ENTRYPOINT ["/cadence-worker"]

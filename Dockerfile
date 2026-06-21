FROM golang:1.23-alpine AS builder
RUN apk add --no-cache ca-certificates git
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o gmock ./cmd/gmock

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /build/gmock /gmock
EXPOSE 8080
ENTRYPOINT ["/gmock"]
CMD ["start", "--port", "8080"]

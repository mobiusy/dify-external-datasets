FROM golang:1.23.4-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /dify-external-datasets

FROM alpine:3.18
WORKDIR /app
COPY --from=builder /dify-external-datasets .
RUN chmod +x /app/dify-external-datasets
EXPOSE 8080
CMD ["/app/dify-external-datasets"]
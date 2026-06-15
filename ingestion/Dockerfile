FROM golang:alpine AS builder
WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o main ./cmd/server/main.go

FROM debian:bookworm-slim
RUN echo 'Acquire::Check-Valid-Until "false";' > /etc/apt/apt.conf.d/99no-check-valid-until
RUN sed -i 's|http://deb.debian.org/debian-security|http://linux-mirror.liara.ir/repository/debian-security|g' /etc/apt/sources.list.d/debian.sources || true && \
    sed -i 's|http://deb.debian.org/debian|http://linux-mirror.liara.ir/repository/debian|g' /etc/apt/sources.list.d/debian.sources || true && \
    sed -i 's|http://deb.debian.org/debian-security|http://linux-mirror.liara.ir/repository/debian-security|g' /etc/apt/sources.list || true && \
    sed -i 's|http://deb.debian.org/debian|http://linux-mirror.liara.ir/repository/debian|g' /etc/apt/sources.list || true

RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /app/main .
EXPOSE 50051
CMD ["./main"]
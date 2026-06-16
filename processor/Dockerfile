FROM golang:alpine AS builder
WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o main ./main.go

FROM alpine:latest
RUN apk add --no-cache tzdata
ENV TZ=Asia/Tehran
WORKDIR /app
COPY --from=builder /app/main .
CMD ["./main"]
# لایه بیلد (Builder)
FROM golang:alpine AS builder
WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct

RUN apk add --no-cache gcc musl-dev

COPY . .
RUN go mod tidy

RUN CGO_ENABLED=1 GOOS=linux go build -a -o main ./main.go

FROM alpine:latest
RUN apk add --no-cache tzdata libc6-compat
ENV TZ=Asia/Tehran
WORKDIR /app
COPY --from=builder /app/main .
CMD ["./main"]
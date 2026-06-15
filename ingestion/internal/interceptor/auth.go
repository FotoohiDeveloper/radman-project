package interceptor

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	redisclient "radman.local/ingestion/internal/redis"
)

func AuthAndRateLimitInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, fmt.Errorf("missing metadata")
	}

	authHeader := md["authorization"]
	if len(authHeader) == 0 {
		return nil, fmt.Errorf("unauthorized: missing token")
	}

	tokenString := strings.TrimPrefix(authHeader[0], "Bearer ")
	secret := []byte(os.Getenv("JWT_SECRET_KEY"))

	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		return secret, nil
	})

	if err != nil || !token.Valid {
		return nil, fmt.Errorf("unauthorized: please login again")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("unauthorized: invalid claims")
	}

	if claims["type"] != "access" {
		return nil, fmt.Errorf("unauthorized: only access tokens are allowed")
	}

	userID := claims["user_id"].(string)

	rdb := redisclient.Client
	rateKey := fmt.Sprintf("rate_limit:%s", userID)
	
	reqCount, err := rdb.Incr(ctx, rateKey).Result()
	if err == nil && reqCount == 1 {
		rdb.Expire(ctx, rateKey, 1*time.Second)
	}

	if reqCount > 3 {
		return nil, fmt.Errorf("rate_limit_exceeded")
	}

	newCtx := context.WithValue(ctx, "user_id", userID)
	return handler(newCtx, req)
}
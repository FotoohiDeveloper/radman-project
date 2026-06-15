package main

import (
	"log"
	"net"

	"google.golang.org/grpc"
	"radman.local/ingestion/internal/interceptor"
	"radman.local/ingestion/internal/redis"
	"radman.local/ingestion/internal/service"
	pb "radman.local/ingestion/proto"
)

func main() {
	redis.InitRedis()

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("❌ failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(interceptor.AuthAndRateLimitInterceptor),
	)

	pb.RegisterIngestionServiceServer(grpcServer, &service.IngestionServer{})

	log.Println("🚀 Radman Ingestion gRPC Server is running on port 50051...")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("❌ failed to serve: %v", err)
	}
}
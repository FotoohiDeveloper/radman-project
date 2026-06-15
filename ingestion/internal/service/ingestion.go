package service

import (
	"context"

	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
	pb "radman.local/ingestion/proto"
	redisclient "radman.local/ingestion/internal/redis"
)

type IngestionServer struct {
	pb.UnimplementedIngestionServiceServer
}

func (s *IngestionServer) SendBatch(ctx context.Context, req *pb.SensorBatchRequest) (*pb.SensorBatchResponse, error) {
	userID, ok := ctx.Value("user_id").(string)
	if !ok {
		return &pb.SensorBatchResponse{Success: false, Error: "unauthorized"}, nil
	}

	payloadBytes, err := proto.Marshal(req)
	if err != nil {
		return &pb.SensorBatchResponse{Success: false, Error: "serialization_failed"}, nil
	}
    
	err = redisclient.Client.XAdd(ctx, &redis.XAddArgs{
		Stream: "stream:visual:raw",
		MaxLen: 100000, 
		Approx: true,
		Values: map[string]interface{}{
			"u": userID,
			"d": payloadBytes,
		},
	}).Err()

	if err != nil {
		return &pb.SensorBatchResponse{Success: false, Error: "database_error"}, nil
	}

	return &pb.SensorBatchResponse{Success: true, Error: ""}, nil
}
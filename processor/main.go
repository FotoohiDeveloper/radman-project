package main

import (
	"context"
	"fmt"
	"log"
	// "os"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
	pb "radman.local/processor/proto"
)

const (
	StreamName    = "stream:visual:raw"
	ConsumerGroup = "processor_group"
	ConsumerName  = "worker_1"
)

func main() {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "redis_db:6379",
		Username: "radman_service",
		Password: "ServicePass2026",
		DB:       0,
	})

	ctx := context.Background()
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("❌ Failed to connect to Redis: %v", err)
	}
	log.Println("✅ Processor connected to Redis successfully!")

	err = rdb.XGroupCreateMkStream(ctx, StreamName, ConsumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		log.Fatalf("❌ Error creating consumer group: %v", err)
	}

	log.Println("🚀 Waiting for radar data...")

	for {
		streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    ConsumerGroup,
			Consumer: ConsumerName,
			Streams:  []string{StreamName, ">"},
			Count:    10,
			Block:    2 * time.Second,
		}).Result()

		if err == redis.Nil {
			continue
		} else if err != nil {
			log.Printf("⚠️ Error reading from stream: %v", err)
			time.Sleep(1 * time.Second)
			continue
		}

		for _, stream := range streams {
			for _, message := range stream.Messages {
				userID := message.Values["u"].(string)
				payloadStr := message.Values["d"].(string)

				payloadBytes := []byte(payloadStr)

				var batch pb.SensorBatchRequest
				if err := proto.Unmarshal(payloadBytes, &batch); err != nil {
					log.Printf("❌ Failed to unmarshal protobuf: %v", err)
					continue
				}

				fmt.Printf("🎯 [NEW BATCH] Target: %s | User: %s | Base Alt: %.2f | Points: %d\n",
					batch.Y.String(), userID, batch.Al, len(batch.P))

				rdb.XAck(ctx, StreamName, ConsumerGroup, message.ID)
			}
		}
	}
}
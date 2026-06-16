package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/uber/h3-go/v4"
	"google.golang.org/protobuf/proto"

	"radman.local/processor/internal/geo"
	pb "radman.local/processor/proto"
)

const (
	StreamName    = "stream:visual:raw"
	ConsumerGroup = "processor_group"
	ConsumerName  = "worker_1"
	H3Resolution  = 6 // ساخت شش‌ضلعی‌هایی به شعاع تقریبی ۳ کیلومتر
)

type TargetCluster struct {
	TargetType pb.TargetType
	Reports    []*pb.SensorBatchRequest
	UserIDs    []string
}

func main() {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "redis_db:6379", // مخصوص محیط داکر
		Username: "radman_service",
		Password: "ServicePass2026",
		DB:       0,
	})

	ctx := context.Background()
	if _, err := rdb.Ping(ctx).Result(); err != nil {
		log.Fatalf("❌ Failed to connect to Redis: %v", err)
	}
	log.Println("✅ AI Processor core activated!")

	rdb.XGroupCreateMkStream(ctx, StreamName, ConsumerGroup, "0")

	for {
		// بافر کردن پیام‌ها در پنجره‌های ۲ ثانیه‌ای
		streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    ConsumerGroup,
			Consumer: ConsumerName,
			Streams:  []string{StreamName, ">"},
			Count:    50,
			Block:    2 * time.Second,
		}).Result()

		if err == redis.Nil {
			continue
		} else if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		clusters := make(map[string]map[pb.TargetType]*TargetCluster)
		var processedIDs []string

		// فاز ۱: خوشه‌بندی گزارش‌ها بر اساس منطقه H3
		for _, stream := range streams {
			for _, message := range stream.Messages {
				userID := message.Values["u"].(string)
				payloadStr := message.Values["d"].(string)

				var batch pb.SensorBatchRequest
				if err := proto.Unmarshal([]byte(payloadStr), &batch); err != nil {
					continue
				}

				// 🌟 حل ارور دوم: دریافت کردن err از تابع H3
				cell, err := h3.LatLngToCell(h3.NewLatLng(batch.La, batch.Lo), H3Resolution)
				if err != nil {
					continue // اگر مختصات نامعتبر بود، این پکت رو رد کن
				}
				cellID := cell.String()

				if _, exists := clusters[cellID]; !exists {
					clusters[cellID] = make(map[pb.TargetType]*TargetCluster)
				}
				if _, exists := clusters[cellID][batch.Y]; !exists {
					clusters[cellID][batch.Y] = &TargetCluster{
						TargetType: batch.Y,
						Reports:    []*pb.SensorBatchRequest{},
						UserIDs:    []string{},
					}
				}

				clusters[cellID][batch.Y].Reports = append(clusters[cellID][batch.Y].Reports, &batch)
				// 🌟 حل ارور اول: اضافه کردن userID به لیست کاربران این خوشه
				clusters[cellID][batch.Y].UserIDs = append(clusters[cellID][batch.Y].UserIDs, userID)
				
				processedIDs = append(processedIDs, message.ID)
			}
		}

		// فاز ۲: محاسبه و رهگیری
		for cellID, targets := range clusters {
			for targetType, cluster := range targets {
				observerCount := len(cluster.Reports)

				fmt.Printf("\n==================================================\n")
				fmt.Printf("🌐 [H3 SECTOR DETECTED] Region ID: %s\n", cellID)
				fmt.Printf("🎯 Target: %s | Observers: %d\n", targetType.String(), observerCount)

				if observerCount == 1 {
					// 🚀 سناریو دوم تو: کاربر تیزبین و تنها!
					report := cluster.Reports[0]
					if len(report.P) > 0 {
						lastPoint := report.P[len(report.P)-1] // استفاده از آخرین نقطه ارسالی کاربر

						agl := geo.DefaultAGL[targetType]
						dist := geo.CalculateDistance(agl, lastPoint.P)
						tLat, tLon := geo.CalculateTargetCoordinates(report.La, report.Lo, dist, lastPoint.Az)

						fmt.Println("⚠️ Status: Low Confidence (Single Observer Altitude Constraint)")
						fmt.Printf("📍 Calculated Position -> Lat: %.6f, Lon: %.6f\n", tLat, tLon)
						fmt.Printf("📏 Estimated Distance from Sensor: %.2f meters\n", dist)
					}
				} else {
					// 🚀 سناریو اول تو: چند کاربر تو یک منطقه (در فاز بعدی مثلث‌بندی می‌شه)
					fmt.Println("🔥 Status: High Confidence (Ready for Multi-Observer Triangulation)")
				}
				fmt.Printf("==================================================\n")
			}
		}

		if len(processedIDs) > 0 {
			rdb.XAck(ctx, StreamName, ConsumerGroup, processedIDs...)
		}
	}
}
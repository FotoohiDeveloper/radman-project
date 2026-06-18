package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/uber/h3-go/v4"
	"google.golang.org/protobuf/proto"

	"radman.local/processor/internal/database"
	"radman.local/processor/internal/geo"
	"radman.local/processor/internal/tracker"
	pb "radman.local/processor/proto"
)

const (
	StreamName    = "stream:visual:raw"
	ConsumerGroup = "processor_group"
	ConsumerName  = "worker_1"
	H3Resolution  = 6
)

type TargetCluster struct {
	TargetType pb.TargetType
	Reports    []*pb.SensorBatchRequest
	UserIDs    []string
}

func main() {
	// ۱. اتصال به دیتابیس مستقل رهگیری
	database.ConnectTrackingDB()

	// ۲. اتصال به ردیس
	rdb := redis.NewClient(&redis.Options{
		Addr:     "redis_db:6379",
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

	// ۳. راه‌اندازی سرویس پاکسازی اهداف رها شده (هر ۱۰ ثانیه چک می‌کند)
	go func() {
		cleanupTicker := time.NewTicker(10 * time.Second)
		for range cleanupTicker.C {
			tracker.CleanStaleTracks()
		}
	}()

	// ۴. راه‌اندازی تپش مرکزی سیستم (تیک ۱ ثانیه‌ای)
	mainTicker := time.NewTicker(1 * time.Second)

	for range mainTicker.C {
		// خواندن پیام‌ها (با بلاک نیم ثانیه‌ای تا در تیک بعدی تاخیر ایجاد نشود)
		streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    ConsumerGroup,
			Consumer: ConsumerName,
			Streams:  []string{StreamName, ">"},
			Count:    100,
			Block:    500 * time.Millisecond,
		}).Result()

		if err != nil && err != redis.Nil {
			continue
		}

		clusters := make(map[string]map[pb.TargetType]*TargetCluster)
		var processedIDs []string

		// فاز بلعیدن و خوشه‌بندی
		for _, stream := range streams {
			for _, message := range stream.Messages {
				userID := message.Values["u"].(string)
				payloadStr := message.Values["d"].(string)

				var batch pb.SensorBatchRequest
				if err := proto.Unmarshal([]byte(payloadStr), &batch); err != nil {
					continue
				}

				cell, err := h3.LatLngToCell(h3.NewLatLng(batch.La, batch.Lo), H3Resolution)
				if err != nil {
					continue
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
				clusters[cellID][batch.Y].UserIDs = append(clusters[cellID][batch.Y].UserIDs, userID)
				processedIDs = append(processedIDs, message.ID)
			}
		}

		// فاز پردازش و ارسال به موتور رهگیری (State Machine)
		for cellID, targets := range clusters {
			for targetType, cluster := range targets {
				observerCount := len(cluster.Reports)
				targetName := targetType.String()

				if observerCount == 1 {
					report := cluster.Reports[0]
					agl := geo.DefaultAGL[targetType]

					// فقط یک بار لاگ می‌ندازیم که دیتای پایه رو گرفتیم
					fmt.Printf("📥 [DATA INGEST] Absorbing %d historical points to calculate vector for %s...\n", len(report.P), targetName)

					for _, point := range report.P {
						dist := geo.CalculateDistance(agl, point.P)
						tLat, tLon := geo.CalculateTargetCoordinates(report.La, report.Lo, dist, point.Az)
						
						// ارسال نقاط به ترکر برای درآوردن سرعت (بدون لاگ اضافی)
						tracker.ProcessNewDetection(targetName, tLat, tLon, agl, "Low", point.T)
					}
				} else {
					// سناریوی چند کاربره (تخمین میانگین)
					var sumLat, sumLon float64
					agl := geo.DefaultAGL[targetType]
					validPoints := 0
					
					// 🌟 یک متغیر برای ذخیره زمان تقریبیِ این گروه از گزارش‌ها
					var batchTimestamp int64 

					for _, report := range cluster.Reports {
						if len(report.P) > 0 {
							lastPoint := report.P[len(report.P)-1]
							dist := geo.CalculateDistance(agl, lastPoint.P)
							lat, lon := geo.CalculateTargetCoordinates(report.La, report.Lo, dist, lastPoint.Az)
							sumLat += lat
							sumLon += lon
							validPoints++
							
							// زمانِ اولین گزارش معتبر رو به عنوان زمانِ کل این گروه (Batch) در نظر می‌گیریم
							if batchTimestamp == 0 {
								batchTimestamp = lastPoint.T
							}
						}
					}

					if validPoints > 0 {
						avgLat := sumLat / float64(validPoints)
						avgLon := sumLon / float64(validPoints)
						
						// 🌟 حل ارور: پارامتر ششم (batchTimestamp) به تابع اضافه شد
						tracker.ProcessNewDetection(targetName, avgLat, avgLon, agl, "High", batchTimestamp)
						fmt.Printf("🔥 [TRACKER INGEST] Multi-Observer (%d) -> Type: %s | Cell: %s\n", validPoints, targetName, cellID)
					}
				}
			}
		}

		// تایید پیام‌ها در ردیس
		if len(processedIDs) > 0 {
			rdb.XAck(ctx, StreamName, ConsumerGroup, processedIDs...)
		}

		tracker.PredictNextStep()
	}
}
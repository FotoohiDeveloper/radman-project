package tracker

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	"radman.local/processor/internal/config"
	"radman.local/processor/internal/database"
)

type LiveTrack struct {
	ID          uuid.UUID
	TargetType  string
	CurrentLat  float64
	CurrentLon  float64
	CurrentAlt  float64
	VelocityX   float64
	VelocityY   float64
	LastUpdated time.Time
	Status      string
}

var (
	GlobalTracks = make(map[uuid.UUID]*LiveTrack)
	mu           sync.Mutex
)

func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000.0
	phi1 := lat1 * math.Pi / 180
	phi2 := lat2 * math.Pi / 180
	deltaPhi := (lat2 - lat1) * math.Pi / 180
	deltaLambda := (lon2 - lon1) * math.Pi / 180

	a := math.Sin(deltaPhi/2)*math.Sin(deltaPhi/2) +
		math.Cos(phi1)*math.Cos(phi2)*math.Sin(deltaLambda/2)*math.Sin(deltaLambda/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

func ProcessNewDetection(targetType string, lat, lon, alt float64, confidence string, pointTimestamp int64) {
	mu.Lock()
	defer mu.Unlock()

	pointTime := time.Unix(pointTimestamp, 0)
	var matchedTrack *LiveTrack
	minDist := config.App.Radar.AssociationRadius

	for _, track := range GlobalTracks {
		if track.TargetType != targetType {
			continue
		}
		dist := haversineDistance(lat, lon, track.CurrentLat, track.CurrentLon)
		if dist < minDist {
			minDist = dist
			matchedTrack = track
		}
	}

	if matchedTrack != nil {
		dt := pointTime.Sub(matchedTrack.LastUpdated).Seconds()
		if dt > 0 {
			matchedTrack.VelocityX = (lon - matchedTrack.CurrentLon) / dt
			matchedTrack.VelocityY = (lat - matchedTrack.CurrentLat) / dt
		}

		matchedTrack.CurrentLat = lat
		matchedTrack.CurrentLon = lon
		matchedTrack.CurrentAlt = alt
		matchedTrack.LastUpdated = pointTime
		matchedTrack.Status = "Active"

		database.DB.Create(&database.TrackPoint{
			TrackID:    matchedTrack.ID,
			Lat:        lat,
			Lon:        lon,
			Alt:        alt,
			Confidence: confidence,
			ReportTime: pointTime,
		})

	} else {
		newID := uuid.New()
		newTrack := &LiveTrack{
			ID:          newID,
			TargetType:  targetType,
			CurrentLat:  lat,
			CurrentLon:  lon,
			CurrentAlt:  alt,
			LastUpdated: pointTime,
			Status:      "Active",
		}
		GlobalTracks[newID] = newTrack

		database.DB.Create(&database.TrackRecord{
			ID:         newID,
			TargetType: targetType,
			Status:     "active",
		})
		database.DB.Create(&database.TrackPoint{
			TrackID:    newID,
			Lat:        lat,
			Lon:        lon,
			Alt:        alt,
			Confidence: confidence,
			ReportTime: pointTime,
		})
	}
}

func CleanStaleTracks() {
	mu.Lock()
	defer mu.Unlock()
	now := time.Now()

	timeoutDuration := time.Duration(config.App.Radar.CoastTimeout) * time.Second

	for id, track := range GlobalTracks {
		if now.Sub(track.LastUpdated) > timeoutDuration {
			delete(GlobalTracks, id)
			database.DB.Model(&database.TrackRecord{}).Where("id = ?", id).Update("status", "dropped")
		} else if now.Sub(track.LastUpdated) > 10*time.Second {
			track.Status = "Coasting"
		}
	}
}

func PredictNextStep() {
	mu.Lock()
	defer mu.Unlock()

	for id, track := range GlobalTracks {
		if track.Status == "Dropped" {
			continue
		}

		if track.VelocityX != 0 || track.VelocityY != 0 {
			track.CurrentLat += track.VelocityY
			track.CurrentLon += track.VelocityX
			track.LastUpdated = track.LastUpdated.Add(1 * time.Second)

			fmt.Printf("🔮 [RADAR PREDICT] 🎯 Target: %s | ID: %s | Lat: %.6f | Lon: %.6f\n",
				track.TargetType, id.String()[:8], track.CurrentLat, track.CurrentLon)
		}
	}
}
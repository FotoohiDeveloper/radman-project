package geo

import (
	"math"
	pb "radman.local/processor/proto"
)

var DefaultAGL = map[pb.TargetType]float64{
	pb.TargetType_UFO:            10000.0,
	pb.TargetType_CRUISE_MISSILE: 100.0,
	pb.TargetType_DRONE:          200.0,
	pb.TargetType_HELICOPTER:     500.0,
	pb.TargetType_FIGHTER_JET:    5000.0,
	pb.TargetType_UAV:            1500.0,
	pb.TargetType_AIRPLANE:       10000.0,
}

const EarthRadius = 6371000.0

func CalculateDistance(agl float64, pitch float32) float64 {
	pitchDeg := float64(pitch)
	
	if pitchDeg < 0 {
		pitchDeg = math.Abs(pitchDeg)
	}
	if pitchDeg < 1 {
		pitchDeg = 1
	}

	pitchRad := pitchDeg * (math.Pi / 180.0)
	return agl / math.Tan(pitchRad)
}

func CalculateTargetCoordinates(userLat, userLon float64, distance float64, azimuth float32) (float64, float64) {
	latRad := userLat * (math.Pi / 180.0)
	lonRad := userLon * (math.Pi / 180.0)
	azRad := float64(azimuth) * (math.Pi / 180.0)

	angularDistance := distance / EarthRadius

	targetLatRad := math.Asin(math.Sin(latRad)*math.Cos(angularDistance) +
		math.Cos(latRad)*math.Sin(angularDistance)*math.Cos(azRad))

	targetLonRad := lonRad + math.Atan2(math.Sin(azRad)*math.Sin(angularDistance)*math.Cos(latRad),
		math.Cos(angularDistance)-math.Sin(latRad)*math.Sin(targetLatRad))

	targetLat := targetLatRad * (180.0 / math.Pi)
	targetLon := targetLonRad * (180.0 / math.Pi)

	return targetLat, targetLon
}
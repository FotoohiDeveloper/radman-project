package geo

import (
	"math"
)

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
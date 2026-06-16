package geo

import (
	"math"
	pb "radman.local/processor/proto"
)

// ارتفاع پیش‌فرض پروازی اهداف از سطح زمین (بر حسب متر)
var DefaultAGL = map[pb.TargetType]float64{
	pb.TargetType_UFO:            10000.0,
	pb.TargetType_CRUISE_MISSILE: 100.0, // موشک‌های کروز معمولاً در کف پرواز می‌کنند
	pb.TargetType_DRONE:          200.0,  // کوادکوپترهای تجاری
	pb.TargetType_HELICOPTER:     500.0,
	pb.TargetType_FIGHTER_JET:    5000.0,
	pb.TargetType_UAV:            1500.0, // پهپادهای شناسایی/نظامی
	pb.TargetType_AIRPLANE:       10000.0,
}

const EarthRadius = 6371000.0 // شعاع تقریبی کره زمین به متر

// محاسبه فاصله افقی پرنده روی زمین بر اساس ارتفاع فرضی و زاویه دست کاربر
func CalculateDistance(agl float64, pitch float32) float64 {
	pitchDeg := float64(pitch)
	
	// جلوگیری از خطای ریاضی (نگاه کردن به پایین یا زاویه صفر)
	if pitchDeg < 0 {
		pitchDeg = math.Abs(pitchDeg)
	}
	if pitchDeg < 1 {
		pitchDeg = 1 // حداقل ۱ درجه برای جلوگیری از مسافت بی‌نهایت
	}

	pitchRad := pitchDeg * (math.Pi / 180.0)
	// فرمول تانژانت: فاصله = ارتفاع / تانژانت زاویه دید
	return agl / math.Tan(pitchRad)
}

// پیدا کردن مختصات جغرافیایی دقیق هدف بر روی کره زمین
func CalculateTargetCoordinates(userLat, userLon float64, distance float64, azimuth float32) (float64, float64) {
	latRad := userLat * (math.Pi / 180.0)
	lonRad := userLon * (math.Pi / 180.0)
	azRad := float64(azimuth) * (math.Pi / 180.0)

	angularDistance := distance / EarthRadius

	// فرمول‌های Haversine برای پیدا کردن نقطه مقصد
	targetLatRad := math.Asin(math.Sin(latRad)*math.Cos(angularDistance) +
		math.Cos(latRad)*math.Sin(angularDistance)*math.Cos(azRad))

	targetLonRad := lonRad + math.Atan2(math.Sin(azRad)*math.Sin(angularDistance)*math.Cos(latRad),
		math.Cos(angularDistance)-math.Sin(latRad)*math.Sin(targetLatRad))

	targetLat := targetLatRad * (180.0 / math.Pi)
	targetLon := targetLonRad * (180.0 / math.Pi)

	return targetLat, targetLon
}
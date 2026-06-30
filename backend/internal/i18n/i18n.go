package i18n

import (
	"embed"
	"encoding/json"
	"log"
	"os"
)

//go:embed locales/*.json
var localeFS embed.FS

var (
	messages map[string]string
	fallback map[string]string
)

// Load reads the locale file for the language set in APP_LANG env var (default: "en")
// and caches it in memory. If the selected language is not English, English is also
// loaded as a fallback for any missing keys.
func Load() {
	lang := os.Getenv("APP_LANG")
	if lang == "" {
		lang = "en"
	}

	data, err := localeFS.ReadFile("locales/" + lang + ".json")
	if err != nil {
		log.Fatalf("❌ Failed to load locale '%s': %v", lang, err)
	}
	if err := json.Unmarshal(data, &messages); err != nil {
		log.Fatalf("❌ Failed to parse locale '%s': %v", lang, err)
	}

	if lang != "en" {
		if enData, err := localeFS.ReadFile("locales/en.json"); err == nil {
			json.Unmarshal(enData, &fallback)
		}
	}

	log.Printf("✅ Locale loaded: %s (%d messages)", lang, len(messages))
}

// T returns the translated message for the given key.
// Falls back to the English locale, then to the key itself if still not found.
func T(key string) string {
	if msg, ok := messages[key]; ok {
		return msg
	}
	if fallback != nil {
		if msg, ok := fallback[key]; ok {
			return msg
		}
	}
	return key
}

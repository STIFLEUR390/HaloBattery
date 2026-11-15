// Package i18n translates the few strings the Go side renders: tray menu,
// tooltip and notifications. The Vue UI carries its own dictionary.
package i18n

import (
	"fmt"
	"os"
	"strings"

	"halobattery/internal/config"
)

// table holds every translated string, keyed by message id then language.
var table = map[string]map[string]string{
	"tray.no_device": {
		"en": "No device detected",
		"fr": "Aucun appareil détecté",
	},
	"tray.open": {
		"en": "Open dashboard",
		"fr": "Ouvrir le tableau de bord",
	},
	"tray.refresh": {
		"en": "Refresh now",
		"fr": "Rafraîchir maintenant",
	},
	"tray.settings": {
		"en": "Settings",
		"fr": "Réglages",
	},
	"tray.quit": {
		"en": "Quit HaloBattery",
		"fr": "Quitter HaloBattery",
	},
	"tray.asleep": {
		"en": "asleep",
		"fr": "en veille",
	},
	"tray.charging": {
		"en": "charging",
		"fr": "en charge",
	},
	// %d = number of devices
	"tray.tooltip": {
		"en": "HaloBattery — %d device(s)",
		"fr": "HaloBattery — %d appareil(s)",
	},
	"notify.low_title": {
		"en": "Battery low",
		"fr": "Batterie faible",
	},
	// %s = device name, %d = level
	"notify.low_body": {
		"en": "%s: %d%%",
		"fr": "%s : %d %%",
	},
	"notify.full_title": {
		"en": "Fully charged",
		"fr": "Charge complète",
	},
	"notify.full_body": {
		"en": "%s is fully charged",
		"fr": "%s est chargé à 100 %",
	},
}

// Resolve turns a config language ("auto", "en", "fr") into a concrete one.
func Resolve(lang string) string {
	if lang == "" || lang == "auto" {
		lang = systemLanguage()
	}
	if _, ok := table["tray.open"][lang]; ok {
		return lang
	}
	return "en"
}

func systemLanguage() string {
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		v := os.Getenv(env)
		if v == "" || v == "C" || v == "POSIX" {
			continue
		}
		v = strings.ToLower(v)
		if i := strings.IndexAny(v, "_.@"); i > 0 {
			v = v[:i]
		}
		return v
	}
	return "en"
}

// T returns the message for lang, falling back to English.
func T(lang, key string) string {
	lang = Resolve(lang)
	if m, ok := table[key]; ok {
		if s, ok := m[lang]; ok {
			return s
		}
		if s, ok := m["en"]; ok {
			return s
		}
	}
	return key
}

// Tf is T with printf-style formatting.
func Tf(lang, key string, args ...any) string {
	return fmt.Sprintf(T(lang, key), args...)
}

// Languages re-exports the configured set so callers need one import.
func Languages() []string { return config.Languages }

package futures

// SettingsModule is the row in core.settings holding Preferences.
const SettingsModule = "futures"

// Preferences is what the user asks the Futures agent to always keep in mind.
type Preferences struct {
	Text string `json:"text"`
}

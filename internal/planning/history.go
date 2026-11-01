package planning

// SettingsModule is the row in core.settings holding Preferences.
const SettingsModule = "frontier"

// Preferences is what the user asks the Frontier agent to always keep in mind.
type Preferences struct {
	Text string `json:"text"`
}

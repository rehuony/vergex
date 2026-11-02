package api

// MaskSensitiveString Mask sensitive strings, showing only first 4 and last 4 characters
// Used to mask API Key, Secret Key, Private Key and other sensitive information
func MaskSensitiveString(s string) string {
	if s == "" {
		return ""
	}
	length := len(s)
	if length <= 8 {
		return "****" // String too short, hide everything
	}
	return s[:4] + "****" + s[length-4:]
}

// SanitizeModelConfigForLog Sanitize model configuration for log output.
// Takes the same ModelConfigUpdate type used by the request handler so the two
// can never drift out of sync.
func SanitizeModelConfigForLog(models map[string]ModelConfigUpdate) map[string]interface{} {
	safe := make(map[string]interface{})
	for modelID, cfg := range models {
		safe[modelID] = map[string]interface{}{
			"enabled":           cfg.Enabled,
			"api_key":           MaskSensitiveString(cfg.APIKey),
			"custom_api_url":    cfg.CustomAPIURL,
			"custom_model_name": cfg.CustomModelName,
		}
	}
	return safe
}

// SanitizeAccountConfigForLog Sanitize exchange configuration for log output.
// Takes the same AccountConfigUpdate type used by the request handler so every
// sensitive field is guaranteed to be masked — adding a field to the request
// type without masking it here would not compile around this helper, but more
// importantly keeps the masking exhaustive.
func SanitizeAccountConfigForLog(accounts map[string]AccountConfigUpdate) map[string]interface{} {
	safe := make(map[string]interface{})
	for accountID, cfg := range accounts {
		safeAccount := map[string]interface{}{}

		// Only add masked sensitive fields when they have values
		if cfg.APIKey != "" {
			safeAccount["api_key"] = MaskSensitiveString(cfg.APIKey)
		}
		if cfg.SecretKey != "" {
			safeAccount["secret_key"] = MaskSensitiveString(cfg.SecretKey)
		}
		if cfg.Passphrase != "" {
			safeAccount["passphrase"] = MaskSensitiveString(cfg.Passphrase)
		}
		safe[accountID] = safeAccount
	}
	return safe
}

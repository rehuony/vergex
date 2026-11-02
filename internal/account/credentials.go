package account

import "strings"

// IsSupportedVenue reports whether trading on the account type is supported.
func IsSupportedVenue(venue string) bool {
	switch strings.ToLower(strings.TrimSpace(venue)) {
	case "binance", "okx":
		return true
	default:
		return false
	}
}

func MissingCredentialFields(venue, apiKey, secretKey, passphrase string) []string {
	switch strings.ToLower(strings.TrimSpace(venue)) {
	case "binance":
		return missingNamedFields(
			namedField{"api_key", apiKey},
			namedField{"secret_key", secretKey},
		)
	case "okx":
		return missingNamedFields(
			namedField{"api_key", apiKey},
			namedField{"secret_key", secretKey},
			namedField{"passphrase", passphrase},
		)
	default:
		return []string{"venue"}
	}
}

type namedField struct {
	name  string
	value string
}

func missingNamedFields(fields ...namedField) []string {
	missing := make([]string, 0, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field.value) == "" {
			missing = append(missing, field.name)
		}
	}
	return missing
}

func IsVisible(account *Account) bool {
	if account == nil {
		return false
	}
	return strings.TrimSpace(string(account.APIKey)) != "" ||
		strings.TrimSpace(string(account.SecretKey)) != "" ||
		strings.TrimSpace(string(account.Passphrase)) != ""
}

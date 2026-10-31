package store

import "strings"

// IsSupportedExchangeType reports whether trading on the exchange type is supported.
func IsSupportedExchangeType(exchangeType string) bool {
	switch strings.ToLower(strings.TrimSpace(exchangeType)) {
	case "binance", "okx":
		return true
	default:
		return false
	}
}

func MissingRequiredExchangeCredentialFields(exchangeType, apiKey, secretKey, passphrase string) []string {
	switch strings.ToLower(strings.TrimSpace(exchangeType)) {
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
		return []string{"exchange_type"}
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

func IsVisibleAIModel(model *AIModel) bool {
	if model == nil {
		return false
	}
	return model.Enabled ||
		strings.TrimSpace(string(model.APIKey)) != "" ||
		strings.TrimSpace(model.CustomAPIURL) != "" ||
		strings.TrimSpace(model.CustomModelName) != ""
}

func IsVisibleExchange(exchange *Exchange) bool {
	if exchange == nil {
		return false
	}
	return exchange.Enabled ||
		strings.TrimSpace(string(exchange.APIKey)) != "" ||
		strings.TrimSpace(string(exchange.SecretKey)) != "" ||
		strings.TrimSpace(string(exchange.Passphrase)) != ""
}

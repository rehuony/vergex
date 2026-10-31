package harness

import (
	"regexp"
)

var sessionID = regexp.MustCompile(`^[a-f0-9-]{16,64}$`)
var credentialText = regexp.MustCompile(`(?i)(api[_ -]?key|secret|password|authorization|\x{5bc6}\x{94a5}|\x{5bc6}\x{7801})\s*[:=]\s*\S+|\bBearer\s+\S+|\bsk-[A-Za-z0-9_-]{16,}`)

func HasCredential(text string) bool { return credentialText.MatchString(text) }

package planning

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

var accountLocks sync.Map

// AccountIdentity derives the account lock key from the exchange API key.
// API keys are account identifiers; different keys for one exchange account
// require operator coordination because the account UID is not exposed here.
func AccountIdentity(venue, key string) string {
	identity := venue + ":" + key
	h := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(h[:])
}
func LockAccount(identity string) func() {
	m, _ := accountLocks.LoadOrStore(identity, &sync.Mutex{})
	lock := m.(*sync.Mutex)
	lock.Lock()
	return lock.Unlock
}

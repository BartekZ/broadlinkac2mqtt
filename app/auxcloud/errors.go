package auxcloud

import (
	"errors"
	"net/http"
	"strings"
)

var (
	ErrNotLoggedIn       = errors.New("aux cloud: not logged in")
	ErrUnauthorized      = errors.New("aux cloud: session expired")
	ErrDeviceSession     = errors.New("aux cloud: device session expired")
	ErrUnknownRegion     = errors.New("aux cloud: unknown region")
	ErrEmptyCredentials  = errors.New("aux cloud: email and password are required")
	ErrDeviceNotFound    = errors.New("aux cloud: device is not found")
	ErrMissingCookie     = errors.New("aux cloud: device cookie or session is missing")
	ErrControlFailed     = errors.New("aux cloud: device control failed")
	errInvalidCiphertext = errors.New("aux cloud: invalid ciphertext")
)

func isAuthError(status int, message string) bool {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return true
	}
	lower := strings.ToLower(message)
	return strings.Contains(lower, "session") ||
		strings.Contains(lower, "login") ||
		strings.Contains(lower, "auth") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "userid")
}

func redactSecret(value string) string {
	if value == "" {
		return ""
	}
	return "[redacted]"
}

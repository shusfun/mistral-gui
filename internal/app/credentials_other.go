//go:build !windows && !darwin

package app

import "errors"

func protectSecret(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	return "", errors.New("credential_store_unsupported")
}

func unprotectSecret(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	return "", errors.New("credential_store_unsupported")
}

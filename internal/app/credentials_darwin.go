//go:build darwin

package app

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <stdlib.h>
#include <string.h>
static OSStatus readCredential(const char *account, UInt32 accountLen, UInt32 *len, void **data) {
    const char *service = "com.shusfun.mistral-gui.credentials";
    return SecKeychainFindGenericPassword(NULL, (UInt32)strlen(service), service, accountLen, account, len, data, NULL);
}
static OSStatus writeCredential(const char *account, UInt32 accountLen, const char *data, UInt32 len) {
    const char *service = "com.shusfun.mistral-gui.credentials";
    return SecKeychainAddGenericPassword(NULL, (UInt32)strlen(service), service, accountLen, account, len, data, NULL);
}
*/
import "C"

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"unsafe"
)

func protectSecret(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	sum := sha256.Sum256([]byte(plain))
	account := hex.EncodeToString(sum[:])
	name := C.CString(account)
	defer C.free(unsafe.Pointer(name))
	secret := C.CString(plain)
	defer C.free(unsafe.Pointer(secret))
	status := C.writeCredential(name, C.UInt32(len(account)), secret, C.UInt32(len(plain)))
	if status != C.errSecSuccess && status != C.errSecDuplicateItem {
		return "", errors.New("credential_store_failed")
	}
	return "keychain:" + account, nil
}

func unprotectSecret(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, "keychain:") {
		if strings.HasPrefix(stored, "dpapi:") {
			return "", errors.New("credential_decrypt_failed")
		}
		return stored, nil // Legacy values migrate on the next save.
	}
	account := strings.TrimPrefix(stored, "keychain:")
	if len(account) != 64 {
		return "", errors.New("credential_decrypt_failed")
	}
	name := C.CString(account)
	defer C.free(unsafe.Pointer(name))
	var size C.UInt32
	var data unsafe.Pointer
	if C.readCredential(name, C.UInt32(len(account)), &size, &data) != C.errSecSuccess {
		return "", errors.New("credential_decrypt_failed")
	}
	defer C.SecKeychainItemFreeContent(nil, data)
	return C.GoStringN((*C.char)(data), C.int(size)), nil
}

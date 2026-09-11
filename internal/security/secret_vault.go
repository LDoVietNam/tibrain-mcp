//go:build windows

package security

import (
	"encoding/base64"
	"unsafe"

	"golang.org/x/sys/windows"
)

// dataBlob mirrors the Windows DATA_BLOB structure used by DPAPI.
type dataBlob struct {
	cbData uint32
	pbData *byte
}

// EncryptDPAPI protects plaintext with Windows DPAPI (current user + machine).
// The returned ciphertext is opaque and bound to this Windows context; it can
// only be decrypted on the same user/machine. Caller must base64-encode the
// result before storing it in TEXT columns.
func EncryptDPAPI(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, nil
	}
	in := &dataBlob{
		cbData: uint32(len(plaintext)),
		pbData: &plaintext[0],
	}
	var out dataBlob
	proc := windows.NewLazySystemDLL("crypt32.dll").NewProc("CryptProtectData")
	r1, _, err := proc.Call(
		uintptr(unsafe.Pointer(in)),
		0,                  // szDataDescr
		0,                  // pOptionalEntropy
		0,                  // pvReserved
		0,                  // pPromptStruct
		uintptr(0x01|0x04), // CRYPTPROTECT_UI_FORBIDDEN | CRYPTPROTECT_LOCAL_MACHINE
		uintptr(unsafe.Pointer(&out)),
	)
	if r1 == 0 {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.pbData)))
	if out.cbData == 0 || out.pbData == nil {
		return nil, nil
	}
	enc := make([]byte, int(out.cbData))
	copy(enc, unsafe.Slice(out.pbData, int(out.cbData)))
	return enc, nil
}

// DecryptDPAPI reverses EncryptDPAPI and returns the original plaintext.
func DecryptDPAPI(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return nil, nil
	}
	in := &dataBlob{
		cbData: uint32(len(ciphertext)),
		pbData: &ciphertext[0],
	}
	var out dataBlob
	proc := windows.NewLazySystemDLL("crypt32.dll").NewProc("CryptUnprotectData")
	r1, _, err := proc.Call(
		uintptr(unsafe.Pointer(in)),
		0,             // szDataDescr
		0,             // pOptionalEntropy
		0,             // pvReserved
		0,             // pPromptStruct
		uintptr(0x01), // CRYPTPROTECT_UI_FORBIDDEN
		uintptr(unsafe.Pointer(&out)),
	)
	if r1 == 0 {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.pbData)))
	if out.cbData == 0 || out.pbData == nil {
		return nil, nil
	}
	plain := make([]byte, int(out.cbData))
	copy(plain, unsafe.Slice(out.pbData, int(out.cbData)))
	return plain, nil
}

// EncryptSecret encrypts a UTF-8 secret and returns base64(ciphertext).
func EncryptSecret(secret string) (string, error) {
	if secret == "" {
		return "", nil
	}
	raw, err := EncryptDPAPI([]byte(secret))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// DecryptSecret reverses EncryptSecret and returns the original UTF-8 string.
func DecryptSecret(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	plain, err := DecryptDPAPI(raw)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

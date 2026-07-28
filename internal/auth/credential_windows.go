//go:build windows

package auth

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
	targetName              = "CodexSkillManager:github.com"
)

type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWrittenLow     uint32
	LastWrittenHigh    uint32
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

var (
	advapi32  = syscall.NewLazyDLL("advapi32.dll")
	credRead  = advapi32.NewProc("CredReadW")
	credWrite = advapi32.NewProc("CredWriteW")
	credFree  = advapi32.NewProc("CredFree")
)

func ReadGitHubToken() (string, error) {
	target, _ := syscall.UTF16PtrFromString(targetName)
	var out *credential
	ok, _, callErr := credRead.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&out)))
	if ok == 0 {
		if callErr == syscall.ERROR_NOT_FOUND {
			return "", nil
		}
		return "", fmt.Errorf("CredReadW: %w", callErr)
	}
	defer credFree.Call(uintptr(unsafe.Pointer(out)))
	if out == nil || out.CredentialBlobSize == 0 {
		return "", nil
	}
	bytes := unsafe.Slice(out.CredentialBlob, int(out.CredentialBlobSize))
	return string(bytes), nil
}

func SaveGitHubToken(token, username string) error {
	target, _ := syscall.UTF16PtrFromString(targetName)
	user, _ := syscall.UTF16PtrFromString(username)
	blob := []byte(token)
	var blobPtr *byte
	if len(blob) > 0 {
		blobPtr = &blob[0]
	}
	c := credential{
		Type: credTypeGeneric, TargetName: target, UserName: user,
		CredentialBlobSize: uint32(len(blob)), CredentialBlob: blobPtr,
		Persist: credPersistLocalMachine,
	}
	ok, _, callErr := credWrite.Call(uintptr(unsafe.Pointer(&c)), 0)
	for i := range blob {
		blob[i] = 0
	}
	if ok == 0 {
		return fmt.Errorf("CredWriteW: %w", callErr)
	}
	return nil
}

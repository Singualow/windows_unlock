//go:build windows

package windowskey

import (
	"fmt"
	"os"
	"testing"

	"github.com/singu/proximity-unlock/internal/protocol"
)

func TestUserCNGKeySignAndDelete(t *testing.T) {
	name := fmt.Sprintf("ProximityUnlock.Test.%d", os.Getpid())
	key, err := OpenOrCreate(name, false)
	if err != nil {
		t.Fatal(err)
	}
	defer key.Close()
	defer key.Delete()
	message := []byte("CNG integration test")
	signature, err := key.Sign(message)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := key.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	if !protocol.VerifyP256(publicKey, message, signature[:]) {
		t.Fatal("CNG signature did not verify")
	}
}

package ibkr_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var testRSAKeyPEM []byte

var _ = BeforeSuite(func() {
	key, genErr := rsa.GenerateKey(rand.Reader, 2048)
	Expect(genErr).ToNot(HaveOccurred())
	pkcs8Bytes, marshalErr := x509.MarshalPKCS8PrivateKey(key)
	Expect(marshalErr).ToNot(HaveOccurred())
	testRSAKeyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Bytes})
})

func TestIBKR(tt *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(tt, "IBKR Suite")
}

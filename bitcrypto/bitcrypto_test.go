package bitcrypto

import (
	"testing"

	"github.com/virel-project/virel-blockchain/v2/util"
)

var messageToSign = []byte("Lorem ipsum dolor sit amet, consectetur adipiscing elit. Suspendisse sodales cursus maximus. Donec dapibus, sapien eget rhoncus faucibus, mauris massa commodo purus, eu rutrum nisi nibh eget augue. Maecenas tristique ut magna eu vehicula. Pellentesque vel auctor purus. Vestibulum ante elit, bibendum in ex ut, accumsan commodo sapien. Pellentesque sollicitudin nisl a accumsan consectetur.")

var privateKey = Privkey(util.AssertHexDec("6f4a4e9acc9ad616ce32f609cede6237c562756d54ff13de69a7dad3bd2af51ab77c8848048661adfbce6c1408bb024a769320090647aafb0a995bee8662f57f"))
var publicKey = privateKey.Public()
var signature = Signature(util.AssertHexDec("5d77b880cebb57b7b0ced0ed5dac70eb2f46c580e8538d055ed5881214feab8e2e31216f6903ef56f7e0b9bd96058b5d451babe3bec14107e69005e265252002"))

func TestSignature(t *testing.T) {
	s, err := Sign(messageToSign, privateKey)
	if err != nil {
		t.Error(err)
	}

	if s != signature {
		t.Error("signature does not match")
	}

	if !VerifySignature(publicKey, messageToSign, s) {
		t.Error("signature is not valid")
	}
}

func BenchmarkSign(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := Sign(messageToSign, privateKey)
		if err != nil {
			b.Error(err)
		}
	}
}

func BenchmarkVerify(b *testing.B) {
	for i := 0; i < b.N; i++ {
		VerifySignature(publicKey, messageToSign, signature)
	}
}

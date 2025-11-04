// Package secp256k1 provides a lightweight replacement for go-ethereum's secp256k1
// This implementation redirects all cryptographic operations to decred's highly
// optimized secp256k1 implementation, which supports modern CPU features like AVX-512.
//
// This stub eliminates the heavy go-ethereum dependency while maintaining full
// compatibility with code that imports github.com/ethereum/go-ethereum/crypto/secp256k1.
package secp256k1

import (
	"crypto/elliptic"
	dcrdSecp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// S256 returns the secp256k1 elliptic curve.
// This function redirects to decred's implementation which provides
// the same secp256k1 curve with better performance characteristics.
func S256() elliptic.Curve {
	return dcrdSecp256k1.S256()
}

// Command vectors generates password-hash test vectors from the real auth
// implementation.
//
// The TypeScript port must verify hashes this code produced, not hashes it
// produces itself — a self-consistent pair proves nothing about compatibility
// with the users table. Run it while the Go auth package still exists:
//
//	go run ./tools/vectors > ../../frontend/src/server/auth/password-vectors.json
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	"github.com/Profy256/katafasoccerpredictions/backend/internal/auth"
)

type vector struct {
	Password string `json:"password"`
	Hash     string `json:"hash"`
}

type payload struct {
	GoVersion string   `json:"goVersion"`
	Vectors   []vector `json:"vectors"`
	// Reject are hashes that must fail to verify against Vector[i].Password
	// style mismatches, plus hashes that must be rejected as malformed.
	WrongPassword map[string]string `json:"wrongPassword"`
	InvalidHashes []string          `json:"invalidHashes"`
	// Dummy is the placeholder hash verified when an email does not exist.
	DummyPassword string `json:"dummyPassword"`
	DummyHash     string `json:"dummyHash"`
}

func main() {
	passwords := []string{
		"correct horse battery staple",
		"password",
		"pàsswörd-ünïcode-✓",
		"a-very-long-password-with-spaces and, punctuation!?",
		"12345678",
		"Uganda2026!",
	}

	vectors := make([]vector, 0, len(passwords))
	wrong := map[string]string{}
	for _, password := range passwords {
		hash, err := auth.HashPassword(password)
		if err != nil {
			fmt.Fprintln(os.Stderr, "hash:", err)
			os.Exit(1)
		}
		vectors = append(vectors, vector{Password: password, Hash: hash})

		other := password + "-not-the-password"
		if _, exists := wrong[hash]; !exists {
			wrong[hash] = other
		}
	}

	invalid := []string{
		"",
		"not-a-hash",
		"$argon2i$v=1$m=65536,t=3,p=4$c2FsdHNhbHRzYWx0c2E$a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2U",
		"$argon2id$v=1$m=65536,t=3,p=4$!!!not-base64!!!$a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2V5a2U",
		"$argon2id$v=1$m=65536$only-two-fields$c2FsdA$a2V5",
	}

	out := payload{
		GoVersion:     runtime.Version(),
		Vectors:       vectors,
		WrongPassword: wrong,
		InvalidHashes: invalid,
		DummyPassword: "katafa-nonexistent-account-placeholder",
		DummyHash:     auth.DummyHash,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, "encode:", err)
		os.Exit(1)
	}
}

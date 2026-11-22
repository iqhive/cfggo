package cfggo

import (
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// editDistanceLimited must agree with the plain editDistance on every pair:
// the exact distance when it is at most the limit, and limit+1 otherwise.
// The banded walk it uses is only sound if this property holds, so it is
// verified by exhaustive small cases and randomized longer ones.
func TestEditDistanceLimitedMatchesPlain(t *testing.T) {
	// Exhaustive over short alphabets and lengths
	letters := "abc"
	for la := 0; la <= 5; la++ {
		for lb := 0; lb <= 5; lb++ {
			for _, a := range allStrings(letters, la) {
				for _, b := range allStrings(letters, lb) {
					checkPair(t, a, b)
				}
			}
		}
	}

	// Randomized longer strings, including equal-length pairs (the worst
	// case the band exists for) and multi-byte runes rendered as bytes
	rng := rand.New(rand.NewSource(1))
	alphabet := "abcdefghij"
	for i := 0; i < 20000; i++ {
		a := randomString(rng, alphabet, rng.Intn(40))
		b := randomString(rng, alphabet, rng.Intn(40))
		if rng.Intn(2) == 0 {
			// force similar lengths so the band path is exercised
			for len(b) < len(a) {
				b += string(alphabet[rng.Intn(len(alphabet))])
			}
		}
		checkPair(t, a, b)
	}
}

func checkPair(t *testing.T, a, b string) {
	t.Helper()
	for _, limit := range []int{0, 1, 2, 3, 5} {
		want := editDistance(a, b)
		exceeds := limit + 1
		if want > limit {
			want = exceeds
		}
		if got := editDistanceLimited(a, b, limit); got != want {
			t.Fatalf("editDistanceLimited(%q, %q, %d) = %d, want %d", a, b, limit, got, want)
		}
	}
}

func allStrings(letters string, n int) []string {
	if n == 0 {
		return []string{""}
	}
	var out []string
	for _, c := range letters {
		for _, tail := range allStrings(letters, n-1) {
			out = append(out, string(c)+tail)
		}
	}
	return out
}

func randomString(rng *rand.Rand, alphabet string, n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteByte(alphabet[rng.Intn(len(alphabet))])
	}
	return sb.String()
}

// A configuration document an attacker can write can contain tens of
// thousands of long unrecognized keys. Every one of them is logged with a
// "did you mean" suggestion computed against every struct field, so with the
// plain quadratic edit distance a 2 MB file of 200-character keys already
// cost seconds and a 10 MB file against a large struct cost minutes — a
// startup denial of service. With the bounded distance the work per pair is
// linear in the key length with a tiny constant, so loading stays proportional
// to the file size. 20k keys load in ~200 ms here; the quadratic version
// needs ~11 s, far past the generous budget below even on a slow machine.
func TestUnrecognizedKeySuggestionsStayFast(t *testing.T) {
	if testing.Short() {
		t.Skip("timed regression guard; skipped in -short mode")
	}

	type fields struct {
		Structure
		F00 func() int `cfggo:"field000"`
		F01 func() int `cfggo:"field001"`
		F02 func() int `cfggo:"field002"`
		F03 func() int `cfggo:"field003"`
		F04 func() int `cfggo:"field004"`
		F05 func() int `cfggo:"field005"`
		F06 func() int `cfggo:"field006"`
		F07 func() int `cfggo:"field007"`
		F08 func() int `cfggo:"field008"`
		F09 func() int `cfggo:"field009"`
	}

	var sb strings.Builder
	sb.WriteString("{")
	for i := 0; i < 20000; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`"unknown`)
		for j := 0; j < 185; j++ {
			sb.WriteByte('x')
		}
		sb.WriteString(strconv.Itoa(i))
		sb.WriteString(`":1`)
	}
	sb.WriteString("}")

	dir := t.TempDir()
	file := dir + "/config.json"
	if err := os.WriteFile(file, []byte(sb.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := &fields{}
	start := time.Now()
	if err := cfg.Init(cfg, WithFileConfig(file), WithoutFlags(), WithoutEnv(), WithIgnoreUnknownVars()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 10*time.Second {
		t.Fatalf("Init with 20k unrecognized keys took %s; suggestion computation is quadratic again", elapsed)
	}
}

package conf

import "testing"

func FuzzDecode(f *testing.F) {
	f.Add([]byte("port=8080\n[database]\nhost=localhost\n"))
	f.Add([]byte("value=before#comment\n"))
	f.Add([]byte("[global]\ndebug=true\n"))
	f.Fuzz(func(t *testing.T, input []byte) {
		_, _ = Decode(input)
	})
}

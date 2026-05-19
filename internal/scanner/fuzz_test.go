package scanner

import "testing"

func FuzzParseLine(f *testing.F) {
	seeds := [][]byte{
		[]byte(`{"type":"user","uuid":"u","sessionId":"s","timestamp":"2026-05-19T12:00:00Z"}`),
		[]byte(`{"type":"assistant","uuid":"u","sessionId":"s","timestamp":"2026-05-19T12:00:01Z","message":{"model":"m","usage":{"input_tokens":1,"output_tokens":1,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`),
		[]byte(``),
		[]byte(`{`),
		[]byte(`{"type":"x"}`),
		[]byte(`{"type":"user","uuid":"","sessionId":"s","timestamp":"2026-05-19T12:00:00Z"}`),
		[]byte(`{"type":"user","uuid":"u","sessionId":"s","timestamp":"bad"}`),
		[]byte("\x00\x01\x02"),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must never panic on arbitrary bytes.
		_, _ = parseLine(data, "fuzz-project")
	})
}

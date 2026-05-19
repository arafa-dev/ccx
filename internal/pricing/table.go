package pricing

import (
	"fmt"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

// Rate is a single pricing row for a model that took effect at EffectiveFrom.
// Rates are denominated in USD per million tokens of the corresponding bucket.
type Rate struct {
	Model              string    `yaml:"model"`
	EffectiveFrom      time.Time `yaml:"effective_from"`
	InputPerMTok       float64   `yaml:"input_per_mtok"`
	OutputPerMTok      float64   `yaml:"output_per_mtok"`
	CacheReadPerMTok   float64   `yaml:"cache_read_per_mtok"`
	CacheCreatePerMTok float64   `yaml:"cache_create_per_mtok"`
}

// fileShape is the on-disk YAML structure.
type fileShape struct {
	LastUpdated time.Time `yaml:"last_updated"`
	Models      []Rate    `yaml:"models"`
}

// Table holds the merged set of rates and answers Cost / LastUpdated queries.
// Construct with NewTable or NewTableFromBytes. Safe for concurrent use after
// construction; the rate table is immutable and the warn-once map is guarded.
type Table struct {
	lastUpdated time.Time
	// rates is keyed by model name; each slice is sorted ascending by
	// EffectiveFrom so a reverse linear scan finds the applicable row fast.
	rates map[string][]Rate
}

// LastUpdated returns the most recent effective_from across all loaded rates.
// It is intended for footer-style "rates as of <date>" UI strings.
func (t *Table) LastUpdated() time.Time { return t.lastUpdated }

// NewTableFromBytes constructs a Table from explicit YAML bytes. Useful for
// tests. Pass nil for userOverride if no override is desired. When both are
// provided, userOverride is deep-merged on top of embedded by model name.
func NewTableFromBytes(embedded, userOverride []byte) (*Table, error) {
	base, err := parseFile(embedded)
	if err != nil {
		return nil, fmt.Errorf("parsing embedded pricing: %w", err)
	}

	if len(userOverride) > 0 {
		over, err := parseFile(userOverride)
		if err != nil {
			return nil, fmt.Errorf("parsing user pricing override: %w", err)
		}
		base = mergeFiles(base, over)
	}

	return buildTable(base), nil
}

// parseFile unmarshals YAML bytes into a fileShape, tolerating an empty input.
func parseFile(data []byte) (fileShape, error) {
	var f fileShape
	if len(data) == 0 {
		return f, nil
	}
	if err := yaml.Unmarshal(data, &f); err != nil {
		return f, err
	}
	return f, nil
}

// mergeFiles layers over on top of base.
func mergeFiles(base, over fileShape) fileShape {
	out := fileShape{LastUpdated: base.LastUpdated, Models: nil}
	if over.LastUpdated.After(out.LastUpdated) {
		out.LastUpdated = over.LastUpdated
	}

	overrideNames := make(map[string]struct{}, len(over.Models))
	for _, r := range over.Models {
		overrideNames[r.Model] = struct{}{}
	}

	for _, r := range base.Models {
		if _, replaced := overrideNames[r.Model]; replaced {
			continue
		}
		out.Models = append(out.Models, r)
	}
	out.Models = append(out.Models, over.Models...)
	return out
}

// buildTable converts a parsed fileShape into the queryable Table form.
func buildTable(f fileShape) *Table {
	rates := make(map[string][]Rate, len(f.Models))
	for _, r := range f.Models {
		rates[r.Model] = append(rates[r.Model], r)
	}
	for k := range rates {
		sort.Slice(rates[k], func(i, j int) bool {
			return rates[k][i].EffectiveFrom.Before(rates[k][j].EffectiveFrom)
		})
	}

	last := f.LastUpdated
	for _, list := range rates {
		for _, r := range list {
			if r.EffectiveFrom.After(last) {
				last = r.EffectiveFrom
			}
		}
	}

	return &Table{
		lastUpdated: last,
		rates:       rates,
	}
}

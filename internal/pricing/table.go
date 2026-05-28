package pricing

import (
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/arafa-dev/ccx/internal/contracts"
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

	warnMu   sync.Mutex
	warnedOn map[string]bool
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
		warnedOn:    make(map[string]bool),
	}
}

// Cost returns the estimated USD cost for usage against model at time ts.
//
// Lookup semantics: the applicable rate is the one whose EffectiveFrom is the
// latest value <= ts. If no rate is effective at ts, Cost returns 0 and nil.
//
// Formula, per bucket: (tokens / 1e6) * rate_per_mtok. The four buckets are
// summed. Result is raw float64; the UI is responsible for rounding.
func (t *Table) Cost(model string, ts time.Time, usage contracts.Usage) (float64, error) {
	rate, ok := t.lookup(model, ts)
	if !ok {
		if !t.hasModel(model) && !isNonBillableModel(model) {
			t.logUnknownOnce(model)
		}
		return 0, nil
	}

	const perMillion = 1_000_000.0
	cost := float64(usage.InputTokens)/perMillion*rate.InputPerMTok +
		float64(usage.OutputTokens)/perMillion*rate.OutputPerMTok +
		float64(usage.CacheReadTokens)/perMillion*rate.CacheReadPerMTok +
		float64(usage.CacheCreateTokens)/perMillion*rate.CacheCreatePerMTok
	return cost, nil
}

// lookup returns the Rate effective at ts for model, or (zero, false) if none.
// The model name is matched exactly first, then against its date-stripped form
// (e.g. "claude-haiku-4-5-20251001" → "claude-haiku-4-5") so dated model ids
// emitted by Claude Code resolve to the undated pricing rows.
func (t *Table) lookup(model string, ts time.Time) (Rate, bool) {
	if rate, ok := t.lookupExact(model, ts); ok {
		return rate, true
	}
	if normalized := stripModelDate(model); normalized != model {
		return t.lookupExact(normalized, ts)
	}
	return Rate{}, false
}

// lookupExact resolves a single key against the rate table with no aliasing.
func (t *Table) lookupExact(model string, ts time.Time) (Rate, bool) {
	list, ok := t.rates[model]
	if !ok || len(list) == 0 {
		return Rate{}, false
	}
	for i := len(list) - 1; i >= 0; i-- {
		if !list[i].EffectiveFrom.After(ts) {
			return list[i], true
		}
	}
	return Rate{}, false
}

// hasModel reports whether a pricing row exists for model under either its
// exact name or its date-stripped alias.
func (t *Table) hasModel(model string) bool {
	if _, ok := t.rates[model]; ok {
		return true
	}
	_, ok := t.rates[stripModelDate(model)]
	return ok
}

// datedModelSuffix matches a trailing "-YYYYMMDD" release-date suffix.
var datedModelSuffix = regexp.MustCompile(`-\d{8}$`)

// stripModelDate removes a trailing "-YYYYMMDD" suffix from a model id. Names
// without such a suffix are returned unchanged.
func stripModelDate(model string) string {
	return datedModelSuffix.ReplaceAllString(model, "")
}

// isNonBillableModel reports whether model is a sentinel that carries no real
// pricing (Claude Code emits "" and "<synthetic>" for non-API turns). These
// are expected and must not trigger an unknown-model warning.
func isNonBillableModel(model string) bool {
	if model == "" {
		return true
	}
	return strings.HasPrefix(model, "<") && strings.HasSuffix(model, ">")
}

// logUnknownOnce emits exactly one slog warning per unknown model name.
func (t *Table) logUnknownOnce(model string) {
	t.warnMu.Lock()
	defer t.warnMu.Unlock()
	if t.warnedOn[model] {
		return
	}
	t.warnedOn[model] = true
	slog.Warn("pricing: unknown model, cost will be reported as 0",
		"model", model)
}

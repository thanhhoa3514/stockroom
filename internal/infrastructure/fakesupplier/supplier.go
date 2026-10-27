package fakesupplier

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/thanhhoa3514/stockroom/internal/domain"
)

//go:embed scenarios/*.json
var scenarios embed.FS

type Supplier struct {
	batches   []domain.Batch
	cursors   map[string]int
	failBatch int
	failed    bool
}

// failBatch is one-based; zero disables the outage for a resumed process.
func New(name string, failBatch int) (*Supplier, error) {
	if strings.ContainsAny(name, "/\\.") {
		return nil, fmt.Errorf("unknown scenario %q", name)
	}
	data, err := scenarios.ReadFile("scenarios/" + name + ".json")
	if err != nil {
		return nil, fmt.Errorf("unknown scenario %q (basic, repeat, late-old, bad-data, outage)", name)
	}
	var raw []struct {
		ID         string          `json:"id"`
		Records    []domain.Record `json:"records"`
		NextCursor string          `json:"next_cursor"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	s := &Supplier{cursors: map[string]int{"": 0}}
	for i, b := range raw {
		if i+1 < len(raw) {
			if b.NextCursor == "" {
				return nil, fmt.Errorf("scenario %q: missing next cursor after batch %s", name, b.ID)
			}
			if _, exists := s.cursors[b.NextCursor]; exists {
				return nil, fmt.Errorf("scenario %q: duplicate cursor %q", name, b.NextCursor)
			}
			s.cursors[b.NextCursor] = i + 1
		} else if b.NextCursor != "" {
			return nil, fmt.Errorf("scenario %q: final batch must have an empty next cursor", name)
		}
		s.batches = append(s.batches, domain.Batch{ID: b.ID, Records: b.Records, NextCursor: b.NextCursor})
	}
	// The unchanged small scenarios still accept their original numeric checkpoints.
	if name != "outage" {
		for i := 1; i < len(raw); i++ {
			s.cursors[strconv.Itoa(i)] = i
		}
	}
	if name == "outage" {
		if failBatch < 0 || failBatch > len(raw) {
			return nil, fmt.Errorf("fail-batch must be between 0 and %d", len(raw))
		}
		s.failBatch = failBatch
	}
	return s, nil
}

func (s *Supplier) FetchBatch(ctx context.Context, cursor string) (domain.Batch, error) {
	if err := ctx.Err(); err != nil {
		return domain.Batch{}, err
	}
	i, ok := s.cursors[cursor]
	if !ok || i >= len(s.batches) {
		return domain.Batch{}, fmt.Errorf("invalid supplier cursor %q for this scenario", cursor)
	}
	if s.failBatch == i+1 && !s.failed {
		s.failed = true
		return domain.Batch{}, fmt.Errorf("fictional supplier outage on batch %d", i+1)
	}
	b := s.batches[i]
	b.Records = append([]domain.Record(nil), b.Records...)
	return b, nil
}

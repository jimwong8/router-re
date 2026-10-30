package behavior

import (
	"sort"
	"strings"
	"sync"
)

// Constants for record kinds.
const (
	KindWeb    = "web"
	KindIM     = "im"
	KindOnline = "online"
	KindApp    = "app"
)

// Record represents a single behavior record.
type Record struct {
	TS       int64             `json:"ts"`
	Kind     string            `json:"kind"`
	MAC      string            `json:"mac"`
	IP       string            `json:"ip"`
	Terminal string            `json:"terminal"`
	Target   string            `json:"target"`
	Detail   string            `json:"detail"`
	Extra    map[string]string `json:"extra,omitempty"`
}

// Exempt defines a rule for exempting certain records from being stored.
type Exempt struct {
	Kind      string `json:"kind"`       // empty means all types
	MatchType string `json:"match_type"` // ip / mac / terminal
	Value     string `json:"value"`
	Comment   string `json:"comment"`
}

// Query represents a query filter for Store.Query.
type Query struct {
	Kind    string
	MAC     string
	IP      string
	Keyword string
	Since   int64
	Until   int64
	Limit   int
	Offset  int
}

// Store stores behavior records with exempt/filter logic.
type Store struct {
	mu      sync.RWMutex
	max     int
	records []Record
	exempt  []Exempt
}

// NewStore creates a new Store with a circular buffer of max entries.
// If max <= 0, the default of 20000 is used.
func NewStore(max int) *Store {
	if max <= 0 {
		max = 20000
	}
	return &Store{
		max:     max,
		records: make([]Record, 0, max),
	}
}

// SetExempt sets the exempt rules. Returns an error if any rule is invalid.
// MatchType must be one of ip/mac/terminal.
// Value must be non-empty.
// If Kind is non-empty, it must be a known type (web/im/online/app).
// Maximum 256 rules allowed.
func (s *Store) SetExempt(list []Exempt) error {
	if len(list) > 256 {
		return errExemptTooMany
	}
	validKinds := map[string]bool{
		KindWeb:    true,
		KindIM:     true,
		KindOnline: true,
		KindApp:    true,
	}
	for _, e := range list {
		if e.MatchType != "ip" && e.MatchType != "mac" && e.MatchType != "terminal" {
			return errExemptInvalidMatchType
		}
		if e.Value == "" {
			return errExemptEmptyValue
		}
		if e.Kind != "" && !validKinds[e.Kind] {
			return errExemptInvalidKind
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.exempt = list
	return nil
}

// Exempts returns a copy of the current exempt rules.
func (s *Store) Exempts() []Exempt {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Exempt, len(s.exempt))
	copy(result, s.exempt)
	return result
}

// IsExempt checks whether a record should be exempted from storage.
// Kind as empty matches all types.
// MAC matching is case-insensitive.
// IP matching is exact.
// Terminal matching is exact on the remark name.
// IsExempt 判断一条记录是否命中免记录规则。
func (s *Store) IsExempt(r Record) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isExemptLocked(r)
}

// isExemptLocked 是 IsExempt 的无锁版本。Add 已经持有写锁，
// 再调 IsExempt 会二次拿 RLock —— sync.RWMutex 不可重入，直接死锁。
func (s *Store) isExemptLocked(r Record) bool {
	for _, e := range s.exempt {
		// If Kind is non-empty in the exempt rule, check it matches
		if e.Kind != "" && e.Kind != r.Kind {
			continue
		}
		switch e.MatchType {
		case "mac":
			// case-insensitive MAC comparison
			if strings.EqualFold(r.MAC, e.Value) {
				return true
			}
		case "ip":
			// exact IP match
			if r.IP == e.Value {
				return true
			}
		case "terminal":
			// exact terminal (remark name) match
			if r.Terminal == e.Value {
				return true
			}
		}
	}
	return false
}

// Add adds records to the store. Records that match IsExempt are skipped.
// If adding exceeds max, the oldest records are dropped (circular buffer).
func (s *Store) Add(rs ...Record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range rs {
		if s.isExemptLocked(r) {
			continue
		}
		s.records = append(s.records, r)
		// Circular buffer: drop oldest if exceeding max
		if s.max > 0 && len(s.records) > s.max {
			s.records = s.records[len(s.records)-s.max:]
		}
	}
}

// Query filters records based on the query criteria.
// Returns the filtered results (sorted by TS descending) and the total count
// after filtering (but before Limit/Offset application).
func (s *Store) Query(q Query) ([]Record, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var filtered []Record
	for _, r := range s.records {
		// Kind filter: empty = all
		if q.Kind != "" && r.Kind != q.Kind {
			continue
		}
		// MAC filter: case-insensitive
		if q.MAC != "" && !strings.EqualFold(r.MAC, q.MAC) {
			continue
		}
		// IP filter: exact match
		if q.IP != "" && r.IP != q.IP {
			continue
		}
		// Keyword filter: case-insensitive substring in Target+Detail+Terminal
		if q.Keyword != "" {
			kwLower := strings.ToLower(q.Keyword)
			combined := strings.ToLower(r.Target + r.Detail + r.Terminal)
			if !strings.Contains(combined, kwLower) {
				continue
			}
		}
		// Since/Until filter (unix seconds, 0 means unlimited)
		if q.Since > 0 && r.TS < q.Since {
			continue
		}
		if q.Until > 0 && r.TS > q.Until {
			continue
		}
		filtered = append(filtered, r)
	}

	// 按 TS 倒序；同一秒内保持写入顺序（SliceStable），
	// 否则同秒多条记录的顺序不确定，翻页会出现重复或漏条。
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].TS > filtered[j].TS })

	total := len(filtered)

	limit := q.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 2000 {
		limit = 2000
	}
	start := q.Offset
	if start < 0 {
		start = 0
	}
	if start >= total {
		return []Record{}, total
	}
	end := start + limit
	if end > total {
		end = total
	}
	return filtered[start:end], total
}

// Stats returns per-kind counts plus a "total" key.
func (s *Store) Stats() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	counts := map[string]int{
		KindWeb:    0,
		KindIM:     0,
		KindOnline: 0,
		KindApp:    0,
	}
	for _, r := range s.records {
		switch r.Kind {
		case KindWeb:
			counts[KindWeb]++
		case KindIM:
			counts[KindIM]++
		case KindOnline:
			counts[KindOnline]++
		case KindApp:
			counts[KindApp]++
		}
		counts[totalKey]++
	}
	return counts
}

// Clear removes all records from the store.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = []Record{}
}

var (
	errExemptTooMany          = err("exempt: too many rules (max 256)")
	errExemptInvalidMatchType = err("exempt: invalid match_type, must be ip/mac/terminal")
	errExemptEmptyValue       = err("exempt: value must be non-empty")
	errExemptInvalidKind      = err("exempt: invalid kind, must be one of web/im/online/app")
)

func err(s string) error {
	return errCustom(s)
}

// errCustom is a custom error type for behavior package.
type errCustom string

func (e errCustom) Error() string {
	return string(e)
}

const totalKey = "total"

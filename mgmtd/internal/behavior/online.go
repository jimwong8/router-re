package behavior

import (
	"sort"
	"sync"
)

// OnlineTracker tracks online/offline status of terminals by MAC address.
type OnlineTracker struct {
	mu   sync.Mutex
	seen map[string]int64 // mac -> lastSeen timestamp
}

// NewOnlineTracker creates a new OnlineTracker.
func NewOnlineTracker() *OnlineTracker {
	return &OnlineTracker{
		seen: make(map[string]int64),
	}
}

// Observe records that a MAC was seen at the given time.
// If this is the first time seeing this MAC, a KindOnline record with Detail="up" is produced.
// If the MAC was already seen, the timestamp is updated and an empty slice is returned.
func (t *OnlineTracker) Observe(mac, ip, terminal string, now int64) []Record {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, ok := t.seen[mac]; ok {
		// Already seen, just update the timestamp
		t.seen[mac] = now
		return nil
	}
	// First time seeing this MAC
	t.seen[mac] = now
	record := Record{
		TS:       now,
		Kind:     KindOnline,
		MAC:      mac,
		IP:       ip,
		Terminal: terminal,
		Detail:   "up",
	}
	return []Record{record}
}

// Offline removes entries whose lastSeen is older than now - timeout.
// For each removed entry, a KindOnline record with Detail="down" is produced.
// Returns the list of generated down records.
func (t *OnlineTracker) Offline(now int64, timeout int64) []Record {
	t.mu.Lock()
	defer t.mu.Unlock()

	downRecords := []Record{}
	// Collect keys to remove (those with lastSeen < now - timeout)
	var removed []string
	for mac, lastSeen := range t.seen {
		if now-lastSeen > timeout {
			removed = append(removed, mac)
			downRecords = append(downRecords, Record{
				TS:     now,
				Kind:   KindOnline,
				MAC:    mac,
				Detail: "down",
			})
		}
	}
	// Remove the expired entries
	for _, mac := range removed {
		delete(t.seen, mac)
	}
	// Sort by MAC for stable output
	sort.Slice(downRecords, func(i, j int) bool {
		return downRecords[i].MAC < downRecords[j].MAC
	})
	return downRecords
}

// Online returns a stably sorted list of currently online MAC addresses.
func (t *OnlineTracker) Online() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	macs := make([]string, 0, len(t.seen))
	for mac := range t.seen {
		macs = append(macs, mac)
	}
	sort.Strings(macs)
	return macs
}

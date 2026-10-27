package lb

import (
	"errors"
	"fmt"
	"hash/fnv"
)

// Pick selects a member according to the configured mode.
//
// weighted uses smooth weighted round-robin (nginx's algorithm): every call
// adds each member's weight to its current score, picks the highest, then
// subtracts the total weight from the winner. state carries the "current"
// scores so successive calls distribute proportionally. This is a real
// algorithm, not random — it avoids bursts on one link.
//
// failover returns the first enabled member.
// per-connection hashes hashKey to a member (same key → same member, sticky).
// per-packet round-robins via a counter in state.
func Pick(mode string, members []Member, state map[string]int, hashKey string) (Member, error) {
	enabled := []Member{}
	for _, m := range members {
		if m.Enabled {
			enabled = append(enabled, m)
		}
	}
	if len(enabled) == 0 {
		return Member{}, errors.New("no enabled members available")
	}
	switch mode {
	case "failover":
		return enabled[0], nil
	case "weighted":
		return pickWeighted(enabled, state)
	case "per-connection":
		return pickSticky(enabled, hashKey)
	case "per-packet":
		return pickRoundRobin(enabled, state)
	default:
		return Member{}, fmt.Errorf("unknown mode %q", mode)
	}
}

func pickWeighted(members []Member, state map[string]int) (Member, error) {
	total := 0
	for _, m := range members {
		total += m.Weight
		if _, ok := state[m.Name]; !ok {
			state[m.Name] = 0
		}
	}
	if total <= 0 {
		return Member{}, errors.New("total weight must be > 0")
	}
	best := members[0]
	for _, m := range members {
		state[m.Name] += m.Weight
		if state[m.Name] > state[best.Name] {
			best = m
		}
	}
	state[best.Name] -= total
	return best, nil
}

func pickSticky(members []Member, hashKey string) (Member, error) {
	h := fnv.New32a()
	h.Write([]byte(hashKey))
	v := h.Sum32()
	return members[int(v)%len(members)], nil
}

func pickRoundRobin(members []Member, state map[string]int) (Member, error) {
	n := len(members)
	idx := state["__rr"]
	idx = (idx + 1) % n
	state["__rr"] = idx
	return members[idx], nil
}

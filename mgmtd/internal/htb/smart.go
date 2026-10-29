package htb

import "fmt"

// PlanSmart builds a dry-run HTB config that evenly splits root_mbit across
// the given terminals. It does not apply anything.
func PlanSmart(root_mbit float64, iface string, terminals []string) (Config, error) {
	if err := ValidateIface(iface); err != nil {
		return Config{}, err
	}
	if root_mbit <= 0 {
		return Config{}, fmt.Errorf("root_mbit must be > 0, got %g", root_mbit)
	}
	if len(terminals) == 0 {
		return Config{}, fmt.Errorf("no terminals")
	}
	per := root_mbit / float64(len(terminals))
	classes := make([]Class, 0, len(terminals))
	for _, t := range terminals {
		name := t
		if len(name) > 4 {
			name = name[len(name)-4:]
		}
		classes = append(classes, Class{
			Name:        name,
			RateMbit:    per,
			CeilMbit:    root_mbit,
			Priority:    4,
			MatchDstIPs: []string{t},
		})
	}
	return Config{
		Iface:        iface,
		RootRateMbit: root_mbit,
		Classes:      classes,
		Enabled:      true,
	}, nil
}

package qos

import "fmt"

// Validate checks the given limits for basic correctness before application.
func Validate(limits []RateLimit) error {
	for _, l := range limits {
		if l.Interface == "" {
			return fmt.Errorf("interface required")
		}
		if l.RateMbps <= 0 {
			return fmt.Errorf("invalid rate: %d", l.RateMbps)
		}
	}
	return nil
}

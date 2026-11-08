//go:build !unix

package resources

// PeakRSS is not available here; the profile leaves it out.
func PeakRSS() int64 { return 0 }

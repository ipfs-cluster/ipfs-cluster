// +build !debug,!silent

package ipfscluster

// These are our default log levels
func init() {
	for f, l := range LoggingFacilities {
		SetFacilityLogLevel(f, l)
	}

	for f, l := range LoggingFacilitiesExtra {
		SetFacilityLogLevel(f, l)
	}
}

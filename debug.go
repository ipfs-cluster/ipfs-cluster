// +build debug,!silent

package ipfscluster

func init() {
	l := "DEBUG"
	SetFacilityLogLevel("*", l)

	// for f, _ := range LoggingFacilities {
	// 	SetFacilityLogLevel(f, l)
	// }
}

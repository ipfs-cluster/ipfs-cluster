// +build !debug,silent

package ipfscluster

func init() {
	l := "CRITICAL"
	SetFacilityLogLevel("*", l)
}

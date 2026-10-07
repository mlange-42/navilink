package navilink

import "strings"

// symbols are the waypoint symbol names, indexed by symbol number.
// The names are close to the ones used by Garmin, but this is not always possible.
// See http://home.online.no/~sigurdhu/MapSource-text.htm for Garmin symbol names.
var symbols = []string{
	"Waypoint",
	"Flag",
	"Residence",
	"Waypoint",
	"Electricity Pylon",
	"Tunnel",
	"Gas Station",
	"Convenience Store",
	"Short Tower",
	"Summit",
	"Swimming Area",
	"Forest",
	"Bridge",
	"Crossing",
	"Fork",
	"Right Turn",
	"Left Turn",
	"Bird",
	"Lodging",
	"Campground",
	"Radio Beacon",
	"Cable Car",
	"Church",
	"Tall Tower",
	"Skiing Area",
	"Marina",
	"Fish",
	"Hunting Area",
	"Lake",
	"Fishing Area",
	"Lighthouse",
	"Beach",
	"Boat Ramp",
	"Bike",
	"Railway",
	"Money",
	"Truck Stop",
	"Scenic Area",
	"Gas Station 2",
	"Bar",
	"Runway",
	"Airport",
	"Medical Facility",
	"Hotel",
	"Parking Area",
}

// SymbolName returns the name of the given symbol number.
// Unknown numbers result in "Waypoint".
func SymbolName(id uint8) string {
	if int(id) < len(symbols) {
		return symbols[id]
	}
	return symbols[0]
}

// SymbolID returns the symbol number for the given name, ignoring case.
// Geocaches are mapped to "Flag". Unknown names result in 0 ("Waypoint").
func SymbolID(name string) uint8 {
	name = strings.TrimSpace(name)
	if strings.EqualFold(name, "Geocache") {
		name = "Flag"
	}
	for i, s := range symbols {
		if strings.EqualFold(s, name) {
			return uint8(i)
		}
	}
	return 0
}

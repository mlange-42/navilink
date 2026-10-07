package navilink

import (
	"encoding/binary"
	"fmt"
)

const (
	// MaxRouteNameLength is the maximum length of route names.
	MaxRouteNameLength = 13
	// MaxRoutePoints is the maximum number of waypoints in a route.
	// Each of the up to 9 subroutes holds 14 waypoint IDs,
	// and the last ID must be followed by a null ID.
	MaxRoutePoints = maxSubroutes*subrouteIDs - 1

	maxSubroutes = 9
	subrouteIDs  = 14
	nullID       = 0xffff
	lastSubroute = 0x7f
)

// Route is a route stored on the device.
// It references the waypoints it consists of by their IDs.
type Route struct {
	ID          uint8
	Name        string // up to 13 characters (A-Z, 0-9 and space)
	WaypointIDs []uint16
}

// decodeRoute decodes a route record (T_ROUTE): a 32-byte header,
// followed by up to 9 subroutes of 32 bytes each.
func decodeRoute(data []byte) (Route, error) {
	if len(data) < recordSize {
		return Route{}, fmt.Errorf("route data too short: %d bytes", len(data))
	}
	le := binary.LittleEndian
	r := Route{
		ID:   data[2],
		Name: cString(data[4:18]),
	}
	for off := recordSize; off+recordSize <= len(data); off += recordSize {
		sub := data[off : off+recordSize]
		for i := range subrouteIDs {
			id := le.Uint16(sub[2+2*i:])
			if id == nullID {
				return r, nil
			}
			r.WaypointIDs = append(r.WaypointIDs, id)
		}
		if sub[30] == lastSubroute {
			break
		}
	}
	return r, nil
}

// encodeRoute encodes a route for upload with PidAddARoute.
// The device assigns the ID, so the ID of the route is ignored.
// Only as many subroutes as needed are encoded.
func encodeRoute(r Route) ([]byte, error) {
	if len(r.WaypointIDs) == 0 {
		return nil, fmt.Errorf("route %s has no waypoints", r.Name)
	}
	if len(r.WaypointIDs) > MaxRoutePoints {
		return nil, fmt.Errorf("route %s has %d waypoints, the maximum is %d", r.Name, len(r.WaypointIDs), MaxRoutePoints)
	}
	if len(r.Name) > MaxRouteNameLength {
		return nil, fmt.Errorf("route name %s is longer than %d characters", r.Name, MaxRouteNameLength)
	}
	le := binary.LittleEndian

	msg := le.AppendUint16(nil, 0x2000) // record type
	msg = append(msg, 0x00, 0x20)       // route ID, reserved
	name := make([]byte, 14)
	copy(name, r.Name)
	msg = append(msg, name...)
	msg = append(msg, make([]byte, 12)...) // reserved
	msg = append(msg, 0x7b, 0x77)          // flag, mark

	// One more ID than waypoints for the terminating null ID.
	numSub := (len(r.WaypointIDs) + subrouteIDs) / subrouteIDs
	for s := range numSub {
		msg = le.AppendUint16(msg, 0x2010) // record type
		for i := range subrouteIDs {
			id := uint16(nullID)
			if idx := s*subrouteIDs + i; idx < len(r.WaypointIDs) {
				id = r.WaypointIDs[idx]
			}
			msg = le.AppendUint16(msg, id)
		}
		tag := byte(0x00)
		if s == numSub-1 {
			tag = lastSubroute
		}
		msg = append(msg, tag, 0x77)
	}
	return msg, nil
}

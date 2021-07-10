package main
import "time"
import "strconv"
import "bytes"
import "github.com/vishvananda/netlink"

// NetlinkRoutes Holds routing table status
type NetlinkRoutes struct {
	MegaLAN *MegaLANClass
	RoutingTable byte
	KnownRoutes map[string]*netlink.Route
}

// Reader reads the UDP socket
func (m *NetlinkRoutes) Reader() {
	for {
		F := netlink.Route{Table: int(m.RoutingTable)}
		Routes4, err4 := netlink.RouteListFiltered(netlink.FAMILY_V4, &F, netlink.RT_FILTER_TABLE)
		Routes6, err6 := netlink.RouteListFiltered(netlink.FAMILY_V6, &F, netlink.RT_FILTER_TABLE)
		if (err4==nil && err6 == nil) {
			ThisTime := make(map[string]bool)
			for x := range Routes4 { if (Routes4[x].Dst != nil) {
				Label := Routes4[x].Dst.String() + "." + strconv.Itoa(Routes4[x].Priority)
				ThisTime[Label] = true
				_, known := m.KnownRoutes[Label]
				if (!known) {
					m.KnownRoutes[Label] = &Routes4[x]
					m.MegaLAN.Channel <- MegaLANMessage{Type: 0x10, Route: m.KnownRoutes[Label]} // Add
				}
				if (bytes.Compare(m.KnownRoutes[Label].Gw, Routes4[x].Gw) != 0 || m.KnownRoutes[Label].LinkIndex != Routes4[x].LinkIndex) {
					m.MegaLAN.Channel <- MegaLANMessage{Type: 0x11, Route: m.KnownRoutes[Label]} // Remove
					m.KnownRoutes[Label] = &Routes4[x]
					m.MegaLAN.Channel <- MegaLANMessage{Type: 0x10, Route: m.KnownRoutes[Label]} // Add
				}
			}}
			for x := range Routes6 { if (Routes6[x].Dst != nil) {
				Label := Routes6[x].Dst.String() + "." + strconv.Itoa(Routes6[x].Priority)
				ThisTime[Label] = true
				_, ok := m.KnownRoutes[Label]
				if (!ok) {
					m.KnownRoutes[Label] = &Routes6[x]
					m.MegaLAN.Channel <- MegaLANMessage{Type: 0x10, Route: m.KnownRoutes[Label]} // Add
				}
				if (bytes.Compare(m.KnownRoutes[Label].Gw, Routes6[x].Gw) != 0 || m.KnownRoutes[Label].LinkIndex != Routes6[x].LinkIndex) {
					m.MegaLAN.Channel <- MegaLANMessage{Type: 0x11, Route: m.KnownRoutes[Label]} // Remove
					m.KnownRoutes[Label] = &Routes6[x]
					m.MegaLAN.Channel <- MegaLANMessage{Type: 0x10, Route: m.KnownRoutes[Label]} // Add
				}
			}}
			for Label := range m.KnownRoutes {
				_, ok := ThisTime[Label]
				if (!ok) {
					m.MegaLAN.Channel <- MegaLANMessage{Type: 0x11, Route: m.KnownRoutes[Label]} // Remove
					delete(m.KnownRoutes, Label)
				}
			}
		}
		time.Sleep(5 * time.Second)
	}
}

// AttachRouteMonitor Attaches to program
func AttachRouteMonitor(RoutingTable byte, MegaLAN *MegaLANClass) *NetlinkRoutes {
	var m NetlinkRoutes
	m.MegaLAN = MegaLAN
	m.RoutingTable = RoutingTable
	m.KnownRoutes = make(map[string]*netlink.Route)
	go m.Reader()
	return &m
}
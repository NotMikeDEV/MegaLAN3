package main
import "net"
import "time"
import "bytes"
import "encoding/binary"
import "container/list"
import "math/rand"
import "strconv"
import "github.com/vishvananda/netlink"

// InstalledRoute Remote route
type InstalledRoute struct {
	IP net.IP
	PrefixLength byte
	Metric uint16
	InstalledRoute *netlink.Route
}

func (R *InstalledRoute) String() string {
	return R.IP.String() + "/" + strconv.Itoa(int(R.PrefixLength)) + "." + strconv.Itoa(int(R.Metric))
}

// Peer holds stuff related to a peer connection
type Peer struct {  
	MegaLAN *MegaLANClass
	Address *net.UDPAddr
	Static bool
	UP bool
	Router bool
	MAC net.HardwareAddr
	LastRecv int64
	LastAdvertise int64
	Handshake []byte
	HandshakeSendTime time.Time
	Latency uint16
	PendingRouteUpdates *list.List
	RouterSentPacket []byte
	RouterSentTime int64
	RouterSentID []byte
	RemoteRoutes map[string]*InstalledRoute
	RemoteIPv4 net.IP
	RemoteIPv6 net.IP
}

// Poll [event] called once per second
func (P *Peer) Poll () {
	if (!P.UP) {
		Buf := [0]byte{}
		P.Handshake = P.MegaLAN.SendUDP(P, 0, Buf[:])
		P.HandshakeSendTime = time.Now()

		if (P.RemoteRoutes != nil) {
			P.ClearRoutes()
		}
	} else {
		if (time.Now().Unix() - P.LastRecv > 30) {
			Buf := [0]byte{}
			P.Handshake = P.MegaLAN.SendUDP(P, 0, Buf[:])
			P.HandshakeSendTime = time.Now()
		}
		if (time.Now().Unix() - P.LastRecv > 45) {
			P.Close()
			if (!P.Static) {
				for i := range MegaLAN.Peers {
					if (P.MegaLAN.Peers[i].UP) {
						P.MegaLAN.RemovePeer(P.Address)
						return
					}
				}
			}
		}
		if (P.LastAdvertise == 0 || time.Now().Unix() - P.LastAdvertise > 300) {
			P.LastAdvertise = time.Now().Unix()
			var Packet = bytes.Buffer{}
			for i := range MegaLAN.Peers {
				if (P.MegaLAN.Peers[i].UP) {
					Packet.Write(P.MegaLAN.Peers[i].Address.IP.To16())
					port := make([]byte, 2)
    				binary.BigEndian.PutUint16(port, uint16(P.MegaLAN.Peers[i].Address.Port))
					Packet.Write(port)
				}
			}
			P.MegaLAN.SendUDP(P, 2, Packet.Bytes())
		}
		if (P.Router) {
			if (P.RouterSentPacket != nil && time.Now().Unix() - P.RouterSentTime > 1) {
				P.RouterSentTime = time.Now().Unix()
				P.RouterSentID = P.MegaLAN.SendUDP(P, 0xA0, P.RouterSentPacket)
			}
			P.SendNextRoutingPacket()
		}
	}
}
// Close Called when peer is garbage collected
func (P *Peer) Close () {
	Debug(2, "Close Peer", P.Address)
	P.ClearRoutes()
	if (P.RemoteIPv4 != nil) {
		Debug(2, "Delete Peer Route via", P.RemoteIPv4)
		var IPv4Route netlink.Route
		IPv4Route.Dst = &net.IPNet{IP: P.RemoteIPv4, Mask: net.CIDRMask(32,32)}
		IPv4Route.Scope = netlink.SCOPE_LINK
		IPv4Route.LinkIndex = P.MegaLAN.NIC.Link.Attrs().Index
		IPv4Route.Protocol = 33
		IPv4Route.Table = P.MegaLAN.RoutingTable
		IPv4Route.Priority = 1
		netlink.RouteDel(&IPv4Route)
	}
	if (P.UP) {
		P.MegaLAN.SendUDP(P, 0xFE, []byte{})
		P.UP = false
	}
}
// ClearRoutes Uninstall all routes
func (P *Peer) ClearRoutes () {
	if (P.RemoteRoutes != nil) {
		Debug(2, "Clear Peer Routes via", P.RemoteIPv4)
		for x := range P.RemoteRoutes {
			netlink.RouteDel(P.RemoteRoutes[x].InstalledRoute)
			Debug(2, "DelRoute", P.RemoteRoutes[x])
			delete(P.RemoteRoutes, x)
		}
		P.RemoteRoutes = nil
	}
}

// AddLocalRoute send route to remote peer
func (P *Peer) AddLocalRoute (Dst *net.IPNet, Priority int) {
	if (P.PendingRouteUpdates == nil) {
		return
	}
	if (len(Dst.IP) == 4) {
		Debug(2, "SEND ROUTE IPv4", Dst.String())
		Packet := bytes.Buffer{}
		Packet.WriteByte(0x01)
		Packet.Write(Dst.IP.To4())
		mask, _ := Dst.Mask.Size()
		Packet.WriteByte(byte(mask))
		prio := make([]byte, 2)
		binary.BigEndian.PutUint16(prio, uint16(Priority))
		Packet.Write(prio)
		P.PendingRouteUpdates.PushBack(Packet)
	} else {
		Debug(2, "SEND ROUTE IPv6", Dst.String())
		Packet := bytes.Buffer{}
		Packet.WriteByte(0x02)
		Packet.Write(Dst.IP.To16())
		mask, _ := Dst.Mask.Size()
		Packet.WriteByte(byte(mask))
		prio := make([]byte, 2)
		binary.BigEndian.PutUint16(prio, uint16(Priority))
		Packet.Write(prio)
		P.PendingRouteUpdates.PushBack(Packet)
	}
}
// RemoveLocalRoute remove route from remote peer
func (P *Peer) RemoveLocalRoute (Dst *net.IPNet, Priority int) {
	if (len(Dst.IP) == 4) {
		Debug(2, "WITHDRAW ROUTE IPv4", Dst.String())
		Packet := bytes.Buffer{}
		Packet.WriteByte(0x03)
		Packet.Write(Dst.IP.To4())
		mask, _ := Dst.Mask.Size()
		Packet.WriteByte(byte(mask))
		prio := make([]byte, 2)
		binary.BigEndian.PutUint16(prio, uint16(Priority))
		Packet.Write(prio)
		P.PendingRouteUpdates.PushBack(Packet)
	} else {
		Debug(2, "WITHDRAW ROUTE IPv6", Dst.String())
		Packet := bytes.Buffer{}
		Packet.WriteByte(0x04)
		Packet.Write(Dst.IP.To16())
		mask, _ := Dst.Mask.Size()
		Packet.WriteByte(byte(mask))
		prio := make([]byte, 2)
		binary.BigEndian.PutUint16(prio, uint16(Priority))
		Packet.Write(prio)
		P.PendingRouteUpdates.PushBack(Packet)
	}
}
// InitRemoteRouter remove route from remote peer
func (P *Peer) InitRemoteRouter () {
	Debug(1, "INIT Router", P.Address)
	P.PendingRouteUpdates = list.New()
	Packet := bytes.Buffer{}
	Packet.WriteByte(0x00)
	P.PendingRouteUpdates.PushBack(Packet)
	for i := range P.MegaLAN.KnownRoutes {
		P.AddLocalRoute(P.MegaLAN.KnownRoutes[i].Dst, P.MegaLAN.KnownRoutes[i].Priority)
	}
}
// SendNextRoutingPacket sends a packet of routing data
func (P *Peer) SendNextRoutingPacket () {
	if (P.RouterSentPacket != nil) {
		return
	}
	if (P.PendingRouteUpdates == nil || P.PendingRouteUpdates.Len() == 0) {
		return
	}
	Payload := bytes.Buffer{}
	var Count = 0
	for Count < 50 && P.PendingRouteUpdates.Len() > 0 {
		Front := P.PendingRouteUpdates.Front()
		Value := Front.Value.(bytes.Buffer)
		Payload.WriteByte(byte(Value.Len()))
		Payload.Write(Value.Bytes())
		P.PendingRouteUpdates.Remove(Front)
		Count++
	}
	Debug(3, "Route Send", "Sent", Count, "packets")
	P.RouterSentPacket = Payload.Bytes()
	P.RouterSentTime = time.Now().Unix()
	P.RouterSentID = P.MegaLAN.SendUDP(P, 0xA0, P.RouterSentPacket)
}
// RoutingACK Called when an ACK packet arrives
func (P *Peer) RoutingACK (PacketID []byte) {
	if (P.RouterSentID != nil && bytes.Compare(P.RouterSentID, PacketID) == 0) {
		P.RouterSentPacket = nil
		P.SendNextRoutingPacket()
		return
	}
}
// RoutingUpdate Called for each update in a routing packet
func (P *Peer) RoutingUpdate (Packet []byte) {
	if (!P.Router) {
		return
	}
	Type := Packet[0]
	if (Type == 0) { // Init/Reset
		P.ClearRoutes()
		P.RemoteRoutes = make(map[string]*InstalledRoute)
		for P.RemoteIPv4 == nil {
			var IPv4 net.IP = net.ParseIP("100.127.0.0")
			IPv4[14] = byte(rand.Intn(256))
			IPv4[15] = byte(rand.Intn(256))
			var Allocated = false
			for x := range P.MegaLAN.Peers {
				if (bytes.Compare(P.MegaLAN.Peers[x].RemoteIPv4, IPv4) == 0) {
					Allocated = true
				}
			}
			if (!Allocated) {
				P.RemoteIPv4 = IPv4
			}
		}
		Debug(2, "Init Router", P.RemoteIPv4, P.RemoteIPv6)
		var IPv4Route netlink.Route
		IPv4Route.Dst = &net.IPNet{IP: P.RemoteIPv4, Mask: net.CIDRMask(32,32)}
		IPv4Route.Scope = netlink.SCOPE_LINK
		IPv4Route.LinkIndex = P.MegaLAN.NIC.Link.Attrs().Index
		IPv4Route.Protocol = 33
		IPv4Route.Table = P.MegaLAN.RoutingTable
		IPv4Route.Priority = 1
		netlink.RouteAdd(&IPv4Route)
	}
	if (P.RemoteRoutes == nil) {
		Debug(2, "ROUTER", "Update before Init")
		return
	}
	if (Type == 1 && len(Packet)>=8) { // IPv4 Add
		var IP net.IP = Packet[1:5]
		var Mask = Packet[5]
		var Metric = binary.BigEndian.Uint16(Packet[6:8])
		Route := InstalledRoute{IP, Mask, Metric, nil}
		P.RemoteRoutes[Route.String()] = &Route

		var IPv4Route netlink.Route
		IPv4Route.Dst = &net.IPNet{IP: Route.IP, Mask: net.CIDRMask(int(Route.PrefixLength),32)}
		IPv4Route.Scope = netlink.SCOPE_UNIVERSE
		IPv4Route.Gw = P.RemoteIPv4
		IPv4Route.LinkIndex = P.MegaLAN.NIC.Link.Attrs().Index
		IPv4Route.Protocol = 33
		IPv4Route.Table = P.MegaLAN.RoutingTable
		IPv4Route.Priority = int(Route.Metric + P.Latency)
		netlink.RouteAdd(&IPv4Route)
		Route.InstalledRoute = &IPv4Route
		Debug(2, "AddRoute", Route)
	} else if (Type == 2 && len(Packet)>=20) { // IPv6 Add
		var IP net.IP = Packet[1:17]
		var Mask = Packet[17]
		var Metric = binary.BigEndian.Uint16(Packet[18:20])
		Route := InstalledRoute{IP, Mask, Metric, nil}
		P.RemoteRoutes[Route.String()] = &Route

		var IPv6Route netlink.Route
		IPv6Route.Dst = &net.IPNet{IP: Route.IP, Mask: net.CIDRMask(int(Route.PrefixLength),128)}
		IPv6Route.Scope = netlink.SCOPE_UNIVERSE
		IPv6Route.Gw = P.RemoteIPv6
		IPv6Route.LinkIndex = P.MegaLAN.NIC.Link.Attrs().Index
		IPv6Route.Protocol = 33
		IPv6Route.Table = P.MegaLAN.RoutingTable
		IPv6Route.Priority = int(Route.Metric + P.Latency)
		netlink.RouteAdd(&IPv6Route)
		Route.InstalledRoute = &IPv6Route
		Debug(2, "AddRoute", Route)
	} else if (Type == 3 && len(Packet)>=8) { // IPv4 Delete
		var IP net.IP = Packet[1:5]
		var Mask = Packet[5]
		var Metric = binary.BigEndian.Uint16(Packet[6:8])
		RouteTemplate := InstalledRoute{IP, Mask, Metric, nil}
		if Route, ok := P.RemoteRoutes[RouteTemplate.String()]; ok {
			netlink.RouteDel(Route.InstalledRoute)
			Debug(2, "DelRoute", Route)
		}
	} else if (Type == 4 && len(Packet)>=20) { // IPv6 Delete
		var IP net.IP = Packet[1:17]
		var Mask = Packet[17]
		var Metric = binary.BigEndian.Uint16(Packet[18:20])
		RouteTemplate := InstalledRoute{IP, Mask, Metric, nil}
		if Route, ok := P.RemoteRoutes[RouteTemplate.String()]; ok {
			netlink.RouteDel(Route.InstalledRoute)
			Debug(2, "DelRoute", Route)
		}
	} else {
		Debug(2, "Routing Packet", Type, len(Packet))
	}
}
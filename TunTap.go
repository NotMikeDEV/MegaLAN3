package main
import "net"
import "bytes"
import "math/rand"
import "encoding/binary"
import "github.com/mistsys/tuntap"
import "github.com/vishvananda/netlink"

// TunTapClass Represents tuntap interface
type TunTapClass struct {  
	FD *tuntap.Interface
	MegaLAN *MegaLANClass
	Link netlink.Link
	MAC net.HardwareAddr
	LinkLocalIPv6 net.IP
}

// Reader Read thread
func (t *TunTapClass) Reader() {  
	for {
		buf := make([]byte, 1536)
		pkt, err := t.FD.ReadPacket(buf)
		if err != nil {
			Debug(0, "Read Error", err)
		} else {
			Msg := MegaLANMessage{Type: 1, Payload: pkt.Body}
			t.MegaLAN.Channel <- Msg
		}
	}
}

// Write write a packet to the interface
func (t *TunTapClass) Write(Packet []byte) {  
	packet := tuntap.Packet{Body: Packet, Protocol: binary.BigEndian.Uint16(Packet[12:14])}
	t.FD.WritePacket(packet)
}

// AddIP Adds an IP Address to the interface
func (t *TunTapClass) AddIP(IP *netlink.Addr) {  
	Debug(1, "Add IP", IP)
	netlink.AddrAdd(t.Link, IP)
}

// AttachTunTap Interface
func AttachTunTap(Name string, MegaLAN *MegaLANClass) *TunTapClass {
	var t TunTapClass
	tun, err := tuntap.Open(Name, tuntap.DevTap)
	if err != nil {
		Error("Read error:", err)
	}
	t.FD = tun
	t.MegaLAN = MegaLAN
	go t.Reader()
	t.Link, err = netlink.LinkByName(tun.Name())
	netlink.LinkSetUp(t.Link)
	netlink.LinkSetMTU(t.Link, 2900)
	t.MAC = t.Link.Attrs().HardwareAddr
	AddrList, err := netlink.AddrList(t.Link, netlink.FAMILY_V6)
	for x := range AddrList {
		Address := AddrList[x].IP
		if (bytes.Compare(Address[0:8], []byte{0xfe, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) == 0) {
			t.LinkLocalIPv6 = Address
		}
	}
	if (t.LinkLocalIPv6 == nil) {
		t.LinkLocalIPv6 = net.ParseIP("fe80::")
		t.LinkLocalIPv6[8] = byte(rand.Intn(256))
		t.LinkLocalIPv6[9] = byte(rand.Intn(256))
		t.LinkLocalIPv6[10] = byte(rand.Intn(256))
		t.LinkLocalIPv6[11] = byte(rand.Intn(256))
		t.LinkLocalIPv6[12] = byte(rand.Intn(256))
		t.LinkLocalIPv6[13] = byte(rand.Intn(256))
		t.LinkLocalIPv6[14] = byte(rand.Intn(256))
		t.LinkLocalIPv6[15] = byte(rand.Intn(256))
		Addr, _ := netlink.ParseAddr(t.LinkLocalIPv6.String() + "/64")
		netlink.AddrAdd(t.Link, Addr)
	}
	Debug(1, "Interface", tun.Name(), t.MAC, t.LinkLocalIPv6)
	return &t
}
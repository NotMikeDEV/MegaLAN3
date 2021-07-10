package main

import "time"
import "strconv"
import "net"
import "os"
import "bytes"
import "github.com/vishvananda/netlink"
import "math/rand"
import "encoding/binary"

// MegaLANClass holds the main app variables
type MegaLANClass struct {  
	NIC *TunTapClass
	UDP *UDPSocketClass
	Peers map[string]*Peer
	MyID []byte
	EthernetPeers map[string]*Peer
	KnownRoutes map[string]*netlink.Route
	Channel chan MegaLANMessage
	RoutingTable int
	SaveFile string
	SaveFileTime time.Time
}

// MegaLANMessage holds a message for the event queue
type MegaLANMessage struct {
	Type byte
	Payload []byte
	Route *netlink.Route
}

// MegaLAN is the main app
var MegaLAN MegaLANClass
// DebugLevel controls debug output
var DebugLevel = 0
// Init initialise object
func (MegaLAN *MegaLANClass) Init() {
	rand.Seed(time.Now().UnixNano())
	MegaLAN.Peers = make(map[string]*Peer)
	MegaLAN.EthernetPeers = make(map[string]*Peer)
	var Buffer = bytes.Buffer{}
	Buffer.WriteByte(byte(rand.Intn(256)))
	Buffer.WriteByte(byte(rand.Intn(256)))
	Buffer.WriteByte(byte(rand.Intn(256)))
	Buffer.WriteByte(byte(rand.Intn(256)))
	MegaLAN.MyID = Buffer.Bytes()
	MegaLAN.Channel = make(chan MegaLANMessage)
	MegaLAN.KnownRoutes = make(map[string]*netlink.Route)
	MegaLAN.RoutingTable = -1
}
// Poll [event] Triggers every 1 second
func (MegaLAN *MegaLANClass) Poll() {
	for i, P := range MegaLAN.Peers {
		Debug(3, "Poll", i)
		P.Poll()
	}
	for i := range MegaLAN.EthernetPeers {
		if (!MegaLAN.EthernetPeers[i].UP) {
			delete(MegaLAN.EthernetPeers, i)
		}
	}
	if (MegaLAN.SaveFile != "" && time.Now().Unix() - MegaLAN.SaveFileTime.Unix() > 60) {
		Debug(1, "Saving cache", MegaLAN.SaveFile)
		MegaLAN.SaveFileTime = time.Now()
		f, err := os.Create(MegaLAN.SaveFile)
		if (err == nil) {
			for x := range MegaLAN.Peers {
				if (!MegaLAN.Peers[x].UP) {
					if (MegaLAN.Peers[x].Static) {
						f.WriteString("Peer " + MegaLAN.Peers[x].Address.IP.String() + " " + strconv.Itoa(MegaLAN.Peers[x].Address.Port) + " STATIC\n")
					} else {
						f.WriteString("Peer " + MegaLAN.Peers[x].Address.IP.String() + " " + strconv.Itoa(MegaLAN.Peers[x].Address.Port) + "\n")
					}
					f.WriteString(" LastRecv " + strconv.Itoa(int(time.Now().Unix() - MegaLAN.Peers[x].LastRecv)) + "\n")
				} else {
					f.WriteString("Peer " + MegaLAN.Peers[x].Address.IP.String() + " " + strconv.Itoa(MegaLAN.Peers[x].Address.Port) + " UP\n")
					f.WriteString(" LastRecv " + strconv.Itoa(int(time.Now().Unix() - MegaLAN.Peers[x].LastRecv)) + "\n")
					if (MegaLAN.Peers[x].Router) {
						f.WriteString(" MAC " + MegaLAN.Peers[x].MAC.String() + "\n")
						f.WriteString(" IPv6 " + MegaLAN.Peers[x].RemoteIPv6.String() + "\n")
						f.WriteString(" IPv4 " + MegaLAN.Peers[x].RemoteIPv4.String() + "\n")
						for y := range MegaLAN.Peers[x].RemoteRoutes {
							f.WriteString(" Route " + y + "\n")
						}
					}
				}
			}
			for x := range MegaLAN.EthernetPeers {
				P := MegaLAN.EthernetPeers[x]
				f.WriteString("Ethernet " + x + " " + P.Address.IP.String() + "/" + strconv.Itoa(P.Address.Port) + "\n")
			}
			f.Close()
		}
	}
}
// PollThread [event] Triggers every 1 second
func (MegaLAN *MegaLANClass) PollThread() {
	for {
		Msg := MegaLANMessage{Type: 0xFF, Payload: nil}
		MegaLAN.Channel <- Msg
		time.Sleep(time.Second)
	}
}
// ReadEthernet [event] new ethernet packet from tuntap interface
func (MegaLAN *MegaLANClass) ReadEthernet(Buffer []byte) {
	var DestinationMAC net.HardwareAddr = Buffer[0:6]
	var SourceMAC net.HardwareAddr = Buffer[6:12]
	var Type uint16 = binary.BigEndian.Uint16(Buffer[12:14])
	if (Type == 0x0806 && len(Buffer) == 42) { // ARP
		var ARPHeader []byte = Buffer[14:22]
		var SenderMAC net.HardwareAddr = Buffer[22:28]
		var SenderIP net.IP = Buffer[28:32]
		var TargetIP net.IP = Buffer[38:42]
		for x := range MegaLAN.Peers {
			if (MegaLAN.Peers[x].RemoteIPv4 != nil && MegaLAN.Peers[x].MAC != nil && bytes.Compare(MegaLAN.Peers[x].RemoteIPv4.To4(), TargetIP) == 0) {
				ReplyPacket := bytes.Buffer{}
				ReplyPacket.Write(SourceMAC) // Destination MAC
				ReplyPacket.Write(MegaLAN.Peers[x].MAC) // Source MAC
				ReplyPacket.Write(Buffer[12:14]) // EtherType
				ARPHeader[7] = 2 // Change opcode (Reply)
				ReplyPacket.Write(ARPHeader) // ARP Packet Main Fields
				ReplyPacket.Write(MegaLAN.Peers[x].MAC) // Sender MAC
				ReplyPacket.Write(MegaLAN.Peers[x].RemoteIPv4.To4()) // Sender IP
				ReplyPacket.Write(SenderMAC) // Target MAC
				ReplyPacket.Write(SenderIP) // Target IP
				MegaLAN.NIC.Write(ReplyPacket.Bytes())
				Debug(3, "ARP", TargetIP)
				return
			}
		}
	}
	Debug(3, "Read Ethernet", len(Buffer), SourceMAC.String(), DestinationMAC.String())
	if Peer, ok := MegaLAN.EthernetPeers[DestinationMAC.String()]; ok {
		MegaLAN.SendUDP(Peer, 0xFF, Buffer)
		return
	}
	for i := range MegaLAN.Peers {
		if (MegaLAN.Peers[i].UP) {
			MegaLAN.SendUDP(MegaLAN.Peers[i], 0xFF, Buffer)
		}
	}
}
// ReadUDP [event] new UDP packet
func (MegaLAN *MegaLANClass) ReadUDP(remote *net.UDPAddr, Buffer []byte) {
	Type := Buffer[8]
	Payload := Buffer[9:]
	SenderID := Buffer[4:8]
	Debug(3, "Read UDP", remote, Type, len(Buffer))
	if (bytes.Compare(SenderID, MegaLAN.MyID) == 0) {
		Debug(3,"SendToSelf", remote, len(Buffer))
		return
	}
	var Peer = MegaLAN.AddPeer(remote)
	if (Type == 0) {
		Debug(1, "INIT", Peer.Address)
		var Send = bytes.Buffer{}
		Send.Write(Buffer[0:4])
		if(MegaLAN.RoutingTable > -1) {
			Send.Write(MegaLAN.NIC.MAC)
			Send.Write(MegaLAN.NIC.LinkLocalIPv6)
		}
		MegaLAN.SendUDP(Peer, 1, Send.Bytes())
	} else if (Type == 1 && Peer.Handshake != nil && len(Payload) >= 4 && bytes.Compare(Payload[0:4], Peer.Handshake) == 0) {
		if (!Peer.UP) {
			for i := range MegaLAN.Peers {
				MegaLAN.Peers[i].LastAdvertise = 0
			}
		}
		Peer.UP = true
		Peer.Router = len(Payload) >= 26
		Peer.LastRecv = time.Now().Unix()
		Peer.Handshake = nil
		Peer.Latency = uint16(time.Since(Peer.HandshakeSendTime).Milliseconds())
		if (Peer.Latency == 0) {
			Peer.Latency = 1
		}
		Debug(1, "INIT_ACK", Peer.Address, Peer.Latency)
		if (MegaLAN.RoutingTable != -1 && Peer.Router && bytes.Compare(Peer.MAC, Payload[4:10]) != 0) {
			Peer.InitRemoteRouter()
		}
		if (Peer.Router) {
			Peer.MAC = Payload[4:10]
			Peer.RemoteIPv6 = Payload[10:26]
		}
	} else if (Type == 2) {
		Debug(1, "ADVERTISE", Peer.Address, len(Payload))
		for len(Payload) >= 18 {
			IP := Payload[0:16]
			Port := binary.BigEndian.Uint16(Payload[16:18])
			Peer := MegaLAN.AddPeer(&net.UDPAddr{Port: int(Port), IP: IP})
			Debug(2, "Advertise", Peer.Address)
			Payload = Payload[18:]
		}
	} else if (Type == 0xA0 && MegaLAN.RoutingTable != -1) {
		Debug(1, "ROUTER", Peer.Address, len(Payload))
		for len(Payload) > 1 {
			Length := int(Payload[0])
			if (Length < len(Payload)) {
				Packet := Payload[1:Length+1]
				Payload = Payload[Length+1:]
				Peer.RoutingUpdate(Packet)
			} else {
				Payload = Payload[0:0]
				Debug(2, "ROUTER PACKET", "Mangled Tail")
			}
		}
		MegaLAN.SendUDP(Peer, 0xA1, Buffer[0:4])
	} else if (Type == 0xA1 && MegaLAN.RoutingTable != -1) {
		Peer.RoutingACK(Payload)
	} else if (Type == 0xFE) {
		Peer.Close()
	} else if (Type == 0xFF) {
		var DestinationMAC net.HardwareAddr = Payload[0:6]
		var SourceMAC net.HardwareAddr = Payload[6:12]
		MegaLAN.EthernetPeers[SourceMAC.String()] = Peer
		Debug(3, "ETHERNET", Peer.Address, len(Payload), SourceMAC.String(), DestinationMAC.String())
		MegaLAN.NIC.Write(Payload)
	}
}
// SendUDP Send to peer
func (MegaLAN *MegaLANClass) SendUDP(Peer *Peer, Type int, Payload []byte) []byte {
	var Packet = bytes.Buffer{}
	Packet.WriteByte(byte(rand.Intn(256)))
	Packet.WriteByte(byte(rand.Intn(256)))
	Packet.WriteByte(byte(rand.Intn(256)))
	Packet.WriteByte(byte(rand.Intn(256)))
	Packet.Write(MegaLAN.MyID)
	Packet.WriteByte(byte(Type))
	Packet.Write(Payload)
	MegaLAN.UDP.Send(Peer.Address, Packet.Bytes())
	Debug(3, "Send UDP", Peer.Address, Type, len(Packet.Bytes()), Packet.Bytes()[0:4])
	return Packet.Bytes()[0:4]
}

// AddPeer Add a remote peer
func (MegaLAN *MegaLANClass) AddPeer(remote *net.UDPAddr) *Peer {
	var PeerAddr string = remote.IP.String() + " " + strconv.Itoa(remote.Port)
	if Peer, ok := MegaLAN.Peers[PeerAddr]; ok {
		return Peer
	}
	var P = &Peer{Address: remote, MegaLAN: MegaLAN}
	Debug(1, "Add Peer", PeerAddr)
	MegaLAN.Peers[PeerAddr] = P
	P.LastRecv = time.Now().Unix()
	return P
}
// GetPeer Get a remote peer object
func (MegaLAN *MegaLANClass) GetPeer(remote *net.UDPAddr) *Peer {
	var PeerAddr string = remote.IP.String() + " " + strconv.Itoa(remote.Port)
	if Peer, ok := MegaLAN.Peers[PeerAddr]; ok {
		return Peer
	}
	return nil
}
// RemovePeer Remove a remote peer
func (MegaLAN *MegaLANClass) RemovePeer(remote *net.UDPAddr) {
	var PeerAddr string = remote.IP.String() + " " + strconv.Itoa(remote.Port)
	if Peer, ok := MegaLAN.Peers[PeerAddr]; ok {
		Peer.Close()
		delete(MegaLAN.Peers, PeerAddr)
	}
}


package main
import "os"
import "fmt"
import "net"
import "strings"
import "strconv"
import "time"
import "bufio"
import "encoding/binary"
import "github.com/vishvananda/netlink"

func main() {
	MegaLAN.Init()
	RoutingTable := -1
	UDPPort := 206
	InterfaceName := ""
	SaveFile := ""
	var InterfaceAddresses []*netlink.Addr
	for i, s := range os.Args[1:] {
		parts := strings.SplitN(s, "=", 2)
		if (parts[0] == "DEBUG") {
			Level, err := strconv.Atoi(parts[1])
			if (err != nil) {
				Error("Invalid Debug Level")
			}
			DebugLevel = Level
		} else if (parts[0] == "PORT") {
			Port, err := strconv.Atoi(parts[1])
			if (err != nil) {
				Error("Invalid Port")
			}
			UDPPort = Port
		} else if (parts[0] == "NIC") {
			InterfaceName = parts[1]
		} else if (parts[0] == "FILE") {
			SaveFile = parts[1]
		} else if (parts[0] == "IP") {
			Addr, err := netlink.ParseAddr(parts[1])
			if (err!=nil) {
				Error("Invalid Interface Address")
			}
			InterfaceAddresses = append(InterfaceAddresses, Addr)
		} else if (parts[0] == "RT") {
			RT, err := strconv.Atoi(parts[1])
			if (err != nil || RT < 0 || RT > 255) {
				Error("Invalid Routing Table")
			}
			RoutingTable = RT
		} else if (parts[0] == "HOST") {
			host := strings.SplitN(parts[1], " ", 2)
			if (len(host) == 2) {
				Port, err := strconv.Atoi(host[1])
				if (err == nil) {
					IP := net.ParseIP(host[0])
					if (IP != nil) {
						Peer := MegaLAN.AddPeer(&net.UDPAddr{Port: Port, IP: IP})
						Peer.Static = true
					}
				} else {
					Error("Invalid Host", host)
				}
			} else {
				IP := net.ParseIP(host[0])
				if (IP != nil) {
					Peer := MegaLAN.AddPeer(&net.UDPAddr{Port: 206, IP: IP})
					Peer.Static = true
				}
			}
		}
		if (len(parts) == 2) {
			Debug(2, "Command Line", i, parts[0], parts[1])
		}
	}
	MegaLAN.NIC = AttachTunTap(InterfaceName, &MegaLAN)
	for i := range InterfaceAddresses {
		MegaLAN.NIC.AddIP(InterfaceAddresses[i])
	}
	MegaLAN.UDP = AttachUDPSocket(UDPPort, &MegaLAN)
	if (RoutingTable > -1) {
		MegaLAN.RoutingTable = RoutingTable
		AttachRouteMonitor(byte(RoutingTable), &MegaLAN)
	}
	if (SaveFile != "") {
		MegaLAN.SaveFile = SaveFile
		MegaLAN.SaveFileTime = time.Now()
		file, err := os.Open(SaveFile)
		if (err == nil) {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				Line := scanner.Text()
				Parts := strings.Split(Line, " ")
				if (Parts[0] == "Peer") {
					p, _ := strconv.Atoi(Parts[2])
					MegaLAN.AddPeer(&net.UDPAddr{Port: p, IP: net.ParseIP(Parts[1])})
				}
    		}
		}
	}
	go MegaLAN.PollThread()
	for {
		msg := <-MegaLAN.Channel
		if (msg.Type == 0) { // Read UDP
			Addr := net.UDPAddr{IP: msg.Payload[0:16], Port: int(binary.BigEndian.Uint16(msg.Payload[16:18])), Zone: ""}
			MegaLAN.ReadUDP(&Addr, msg.Payload[18:])
		} else if (msg.Type == 1) { // Read Ethernet
			MegaLAN.ReadEthernet(msg.Payload)
		} else if (msg.Type == 0x10) { // AddRoute
			if (msg.Route.LinkIndex != MegaLAN.NIC.Link.Attrs().Index) {
				Debug(1, "AddRoute", msg.Route)
				Label := msg.Route.Dst.String() + "." + strconv.Itoa(msg.Route.Priority)
				_, ok := MegaLAN.KnownRoutes[Label]
				if (!ok) {
					MegaLAN.KnownRoutes[Label] = msg.Route
				}
				for x := range MegaLAN.Peers {
					if (MegaLAN.Peers[x].Router) {
						MegaLAN.Peers[x].AddLocalRoute(msg.Route.Dst, msg.Route.Priority)
					}
				}
			}
		} else if (msg.Type == 0x11) { // RemoveRoute
			if (msg.Route.LinkIndex != MegaLAN.NIC.Link.Attrs().Index) {
				Debug(1, "RemoveRoute", msg.Route)
				Label := msg.Route.Dst.String() + "." + strconv.Itoa(msg.Route.Priority)
				delete(MegaLAN.KnownRoutes, Label)
				for x := range MegaLAN.Peers {
					if (MegaLAN.Peers[x].Router) {
						MegaLAN.Peers[x].RemoveLocalRoute(msg.Route.Dst, msg.Route.Priority)
					}
				}
			}
		} else if (msg.Type == 0xFF) { // Poll
			MegaLAN.Poll()
		}
	}
}

// Debug Output
func Debug(Level int, Event string, Text ...interface{}) {
	if (Level <= DebugLevel) {
		if (Level != 0) {
			fmt.Printf("[%s:%d] ", Event, Level)
		} else {
			fmt.Printf("[%s] ", Event)
		}
		fmt.Println(Text...)
	}
}

// Error Occurred
func Error(Text ...interface{}) {
	Debug(0, "ERROR", Text...)
	os.Exit(1)
	for {}
}

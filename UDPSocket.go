package main

import "net"
import "bytes"
import "encoding/binary"

// UDPSocketClass represents a UDP socket
type UDPSocketClass struct {  
	MegaLAN *MegaLANClass
	UDP *net.UDPConn
}

// Reader UDP socket recv thread
func (u *UDPSocketClass) Reader() {
	for {
		message := make([]byte, 10000)
		rlen, remote, err := u.UDP.ReadFromUDP(message[:])
		if err != nil {
			Error("Error Reading UDP Socket")
		} else {
			var Buffer = bytes.Buffer{}
			Buffer.Write(remote.IP.To16())
			port := make([]byte, 2)
				binary.BigEndian.PutUint16(port, uint16(remote.Port))
				Buffer.Write(port)
			Buffer.Write(message[0:rlen])
			Msg := MegaLANMessage{Type: 0, Payload: Buffer.Bytes()}
			MegaLAN.Channel <- Msg
		}
	}
}

// Send UDP packet
func (u *UDPSocketClass) Send(addr *net.UDPAddr, buffer []byte) {
	u.UDP.WriteMsgUDP(buffer, nil, addr)
}

// AttachUDPSocket attaches UDP to MegaLAN
func AttachUDPSocket(Port int, MegaLAN *MegaLANClass) *UDPSocketClass {
	var u UDPSocketClass
	u.MegaLAN = MegaLAN
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: Port,
		IP:   net.ParseIP("0.0.0.0"),
	})
	if err != nil {
		Error(err)
	}
	u.UDP = udpConn
	Debug(0, "UDP Listening", udpConn.LocalAddr().String())
	go u.Reader()
	return &u
}
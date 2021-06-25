package main

import "net"
import "io"
import "bytes"
import "encoding/binary"
import "crypto/sha256"
import "crypto/aes"
import "crypto/cipher"
import "crypto/rand"

// UDPSocketClass represents a UDP socket
type UDPSocketClass struct {  
	MegaLAN *MegaLANClass
	UDP *net.UDPConn
	Crypto cipher.Block
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
			if rlen > aes.BlockSize {
				iv := message[:aes.BlockSize]
				cipherText := message[aes.BlockSize:]
				stream := cipher.NewCFBDecrypter(u.Crypto, iv)
				stream.XORKeyStream(cipherText, cipherText)
				Buffer.Write(cipherText[0:rlen - aes.BlockSize])
				Msg := MegaLANMessage{Type: 0, Payload: Buffer.Bytes()}
				MegaLAN.Channel <- Msg
			}
		}
	}
}

// Send UDP packet
func (u *UDPSocketClass) Send(addr *net.UDPAddr, buffer []byte) {
	cipherText := make([]byte, aes.BlockSize+len(buffer))
	iv := cipherText[:aes.BlockSize]
	io.ReadFull(rand.Reader, iv)
	stream := cipher.NewCFBEncrypter(u.Crypto, iv)
	stream.XORKeyStream(cipherText[aes.BlockSize:], buffer)
	u.UDP.WriteMsgUDP(cipherText, nil, addr)
}

// AttachUDPSocket attaches UDP to MegaLAN
func AttachUDPSocket(Port int, Password string, MegaLAN *MegaLANClass) *UDPSocketClass {
	var u UDPSocketClass
	u.MegaLAN = MegaLAN
	var Key [32]byte = sha256.Sum256([]byte(Password))
	u.Crypto, _ = aes.NewCipher(Key[:])
	Debug(1, "Password", Password)
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
package mysql

import (
    "io"
    "crypto/sha1"
    //"fmt"
)
type HandShakePacket struct {
    PacketHeader
    ProtoVer        byte
    ServerVer       string
    ConnId          uint32
    CharacterSet    byte
    StatusFlags     uint16
    CapabilityFlags uint32
    AuthString      string
    AuthPluginName  string
}

type AuthPacket struct {
     PacketHeader
/*
http://dev.mysql.com/doc/internals/en/connection-phase-packets.html#packet-Protocol::HandshakeResponse
4              capability flags, CLIENT_PROTOCOL_41 always set
4              max-packet size
1              character set
string[23]     reserved (all [0])
string[NUL]    username
  if capabilities & CLIENT_PLUGIN_AUTH_LENENC_CLIENT_DATA {
lenenc-int     length of auth-response
string[n]      auth-response
  } else if capabilities & CLIENT_SECURE_CONNECTION {
1              length of auth-response
string[n]      auth-response
  } else {
string[NUL]    auth-response
  }
  if capabilities & CLIENT_CONNECT_WITH_DB {
string[NUL]    database
  }
  if capabilities & CLIENT_PLUGIN_AUTH {
string[NUL]    auth plugin name
  }
*/  
    //CLIENT_SECURE_CONNECTION only
    CapabilityFlags     uint32
    MaxPacketSize       uint32
    CharacterSet        byte
    //[23]byte
    Username            string
    AuthResponse        string
    Database            string
    AuthPluginName      string
}


func (self *HandShakePacket) FromBuffer(buffer []byte) (int, error) {
    self.ProtoVer = uint8(buffer[0])
    if self.ProtoVer == 10 {
        return handShakeV10(buffer, self)
    } else if self.ProtoVer == 9 {
        return handShakeV9(buffer, self)
    } else {
        return 0, PROTOCOL_NOT_SUPPORTED
    }
}

func (self *HandShakePacket) ToBuffer(buffer []byte) (writen int, err error) {
    // only support V10
    return
}

func ReadHandShake(reader io.Reader, buffer []byte) (handshake HandShakePacket, err error) {
    err = ReadPacketFrom(&handshake, reader, buffer)
    if err != nil {
       return 
    }
    if int(handshake.PacketLength) > len(buffer) {
        err = BAD_HANDSHAKE_PACKET //handshake packet too big
        return
    }
    if len(handshake.AuthString) != 20 {
        err = BAD_HANDSHAKE_PACKET
    }
    if handshake.PacketSeq != 0 {
        err = BAD_HANDSHAKE_PACKET
        return
    }
    return
}


func handShakeV10(buffer []byte, handshake *HandShakePacket) (read int, err error) {
/*
http://dev.mysql.com/doc/internals/en/connection-phase-packets.html#packet-Protocol::HandshakeV10
1              [0a] protocol version
string[NUL]    server version
4              connection id
string[8]      auth-plugin-data-part-1
1              [00] filler
2              capability flags (lower 2 bytes)
  if more data in the packet:
1              character set
2              status flags
2              capability flags (upper 2 bytes)
  if capabilities & CLIENT_PLUGIN_AUTH {
1              length of auth-plugin-data
  } else {
1              [00]
  }
string[10]     reserved (all [00])
  if capabilities & CLIENT_SECURE_CONNECTION {
string[$len]   auth-plugin-data-part-2 ($len=MAX(13, length of auth-plugin-data - 8))
  if capabilities & CLIENT_PLUGIN_AUTH {
string[NUL]    auth-plugin name
  }
*/
    var p = 1
    var nslen int
    var ns NullString
    nslen, _ = ns.FromBuffer(buffer[p:])
    handshake.ServerVer = string(ns)
    if int(handshake.PacketLength) - nslen < 24 {
        return 0, BAD_HANDSHAKE_PACKET
    }
    p += nslen
    handshake.ConnId = ENDIAN.Uint32(buffer)
    if buffer[p+12] != '\x00' {
        return 0, BAD_HANDSHAKE_PACKET
    }
    handshake.CapabilityFlags = uint32(ENDIAN.Uint16(buffer[p+13:]))
    if len(buffer) > p+15 {
        handshake.CharacterSet = buffer[p+15]
        handshake.StatusFlags = ENDIAN.Uint16(buffer[p+16:])
        handshake.CapabilityFlags += (uint32(ENDIAN.Uint16(buffer[p+18:])) << 16)
    }
    authPluginDataLength := int(buffer[p+20])
    if (handshake.CapabilityFlags & CLIENT_PLUGIN_AUTH) == 0 {
        if authPluginDataLength != 0 {
            return 0, BAD_HANDSHAKE_PACKET    
        }
    }
    if (handshake.CapabilityFlags & CLIENT_SECURE_CONNECTION) != 0 {
        copy(buffer[p+23:p+31], buffer[p+4:p+12])
        handshake.AuthString = string(buffer[p+23:p+22+authPluginDataLength])
    }
    if (handshake.CapabilityFlags & CLIENT_PLUGIN_AUTH) != 0 {
        nslen, _ = ns.FromBuffer(buffer[p+23+authPluginDataLength:])
        handshake.AuthPluginName = string(ns)
    }
    read = p+23+authPluginDataLength+nslen
    return
}

func handShakeV9(buffer []byte, handshake *HandShakePacket) (read int, err error) {
/*
http://dev.mysql.com/doc/internals/en/connection-phase-packets.html#packet-Protocol::HandshakeV9
1              [09] protocol_version
string[NUL]    server_version
4              connection_id
string[NUL]    scramble
*/
    return
}

func (self *AuthPacket) ToBuffer(buffer []byte) (writen int, err error) {
    var p int
    if (self.CapabilityFlags & RELAY_CLIENT_CAP) == 0 {
        err = SERVER_CAPABILITY_NOT_SUFFICIENT
        return
    }
    ENDIAN.PutUint32(buffer[0:], self.CapabilityFlags)
    ENDIAN.PutUint32(buffer[4:], self.MaxPacketSize)
    buffer[5] = self.CharacterSet
    copy(buffer[32:], []byte(self.Username))
    p = 32 + len(self.Username)
    buffer[p] = '\x00'
    p+= 1
    authResponseLength := len(self.AuthResponse)
    buffer[p] = byte(authResponseLength)
    p+= 1
    copy(buffer[p:p+authResponseLength], []byte(self.AuthResponse))
    p+= authResponseLength
    if self.CapabilityFlags & CLIENT_CONNECT_WITH_DB != 0 {
        copy(buffer[p:], []byte(self.Database))
        p+= len(self.Database)
        buffer[p] = '\x00'
        p+= 1
    }
    if self.CapabilityFlags & CLIENT_PLUGIN_AUTH != 0 {
        copy(buffer[p:], []byte(self.AuthPluginName))
        p+= len(self.AuthPluginName)
        buffer[p] = '\x00'
        p+= 1
    }
    writen = p
    return
}

func buildAuthPacket(username string, password string, handshake HandShakePacket) (authPacket AuthPacket, err error) {
    if (handshake.CapabilityFlags & RELAY_CLIENT_CAP) != RELAY_CLIENT_CAP {
        err = SERVER_CAPABILITY_NOT_SUFFICIENT
        return
    }
    // TODO: check CapabilityFlags
    authPacket.CapabilityFlags = RELAY_CLIENT_CAP //handshake.CapabilityFlags
    authPacket.MaxPacketSize   = 0
    authPacket.CharacterSet    = handshake.CharacterSet
    authPacket.Username        = username
    authPacket.AuthResponse    = authResponse(handshake.AuthString, password)
    authPacket.AuthPluginName  = handshake.AuthPluginName
    
    authPacket.PacketSeq = handshake.PacketSeq + 1
    return
}

func authResponse(authString string, password string) string {
    t1 := sha1.Sum([]byte(password))
    t2 := sha1.Sum(t1[:])
    t3 := sha1.Sum([]byte(authString + string(t2[:])))
    for i := range(t1) {
        t3[i] = t1[i] ^ t3[i]
    }
    return string(t3[:])
}


func Auth(authPacket AuthPacket, readWriter io.ReadWriter, buffer []byte) (ret OkPacket, err error) {
    err = WritePacketTo(&authPacket, readWriter, buffer)
    packet, err := ReadGenericResponsePacket(readWriter, buffer)
    if err != nil {
        return
    }
    if packet.PacketSeq != authPacket.PacketSeq + 1 {
        err = PACKET_SEQ_NOT_CORRECT
        return
    }
    ret, err = packet.ToOk()
    return
}

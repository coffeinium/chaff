package dpi

import (
	"net"
	"net/netip"
	"strings"
)

const (
	protoTCP = 6
	protoUDP = 17
)

type Packet struct {
	SrcAddr netip.Addr
	DstAddr netip.Addr
	Proto   uint8
	SrcPort uint16
	DstPort uint16
	Flags   uint8
	Payload []byte
}

func Parse(raw []byte) (Packet, bool) {
	b := raw
	if len(b) >= 14 {
		if v := b[0] >> 4; v != 4 && v != 6 {
			et := uint16(b[12])<<8 | uint16(b[13])
			off := 14
			for et == 0x8100 && len(b) >= off+4 {
				et = uint16(b[off+2])<<8 | uint16(b[off+3])
				off += 4
			}
			if et == 0x0800 || et == 0x86DD {
				b = b[off:]
			}
		}
	}
	if len(b) < 1 {
		return Packet{}, false
	}
	switch b[0] >> 4 {
	case 4:
		return parseV4(b)
	case 6:
		return parseV6(b)
	}
	return Packet{}, false
}

func parseV4(b []byte) (Packet, bool) {
	if len(b) < 20 {
		return Packet{}, false
	}
	ihl := int(b[0]&0x0f) * 4
	if ihl < 20 || len(b) < ihl {
		return Packet{}, false
	}
	p := Packet{Proto: b[9]}
	p.SrcAddr, _ = netip.AddrFromSlice(b[12:16])
	p.DstAddr, _ = netip.AddrFromSlice(b[16:20])

	return parseL4(p, b[ihl:])
}

func parseV6(b []byte) (Packet, bool) {
	if len(b) < 40 {
		return Packet{}, false
	}
	p := Packet{Proto: b[6]}
	p.SrcAddr, _ = netip.AddrFromSlice(b[8:24])
	p.DstAddr, _ = netip.AddrFromSlice(b[24:40])
	return parseL4(p, b[40:])
}

func parseL4(p Packet, l4 []byte) (Packet, bool) {
	switch p.Proto {
	case protoTCP:
		if len(l4) < 20 {
			return p, false
		}
		p.SrcPort = uint16(l4[0])<<8 | uint16(l4[1])
		p.DstPort = uint16(l4[2])<<8 | uint16(l4[3])
		p.Flags = l4[13]
		off := int(l4[12]>>4) * 4
		if off >= 20 && len(l4) >= off {
			p.Payload = l4[off:]
		}
		return p, true
	case protoUDP:
		if len(l4) < 8 {
			return p, false
		}
		p.SrcPort = uint16(l4[0])<<8 | uint16(l4[1])
		p.DstPort = uint16(l4[2])<<8 | uint16(l4[3])
		p.Payload = l4[8:]
		return p, true
	}
	return p, false
}

// SrcMAC возвращает MAC источника, если кадр содержит ethernet-заголовок
// (иначе пусто — данные начинаются сразу с IP).
func SrcMAC(raw []byte) string {
	if len(raw) >= 14 && raw[0]>>4 != 4 && raw[0]>>4 != 6 {
		return net.HardwareAddr(raw[6:12]).String()
	}
	return ""
}

func NormalizeHost(h string) string {
	h = strings.TrimSpace(h)
	if i := strings.LastIndexByte(h, ':'); i >= 0 && isDigits(h[i+1:]) {
		h = h[:i]
	}
	h = strings.TrimSuffix(h, ".")
	return strings.ToLower(h)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

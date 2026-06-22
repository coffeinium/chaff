package dpi

const (
	extServerName = 0x0000
	extECH        = 0xfe0d
)

type TLSHello struct {
	SNI    string
	HasECH bool
}

func ParseClientHello(b []byte) (TLSHello, bool) {
	var h TLSHello

	if len(b) < 5 || b[0] != 0x16 {
		return h, false
	}
	recLen := int(b[3])<<8 | int(b[4])
	rec := b[5:]
	if len(rec) > recLen {
		rec = rec[:recLen]
	}

	if len(rec) < 4 || rec[0] != 0x01 {
		return h, false
	}
	hsLen := int(rec[1])<<16 | int(rec[2])<<8 | int(rec[3])
	body := rec[4:]
	if len(body) > hsLen {
		body = body[:hsLen]
	}

	if len(body) < 34 {
		return h, false
	}
	p := 34

	if p+1 > len(body) {
		return h, false
	}
	p += 1 + int(body[p])

	if p+2 > len(body) {
		return h, false
	}
	p += 2 + (int(body[p])<<8 | int(body[p+1]))

	if p+1 > len(body) {
		return h, false
	}
	p += 1 + int(body[p])

	if p+2 > len(body) {
		return h, true
	}
	extLen := int(body[p])<<8 | int(body[p+1])
	p += 2
	end := p + extLen
	if end > len(body) {
		end = len(body)
	}
	for p+4 <= end {
		et := int(body[p])<<8 | int(body[p+1])
		el := int(body[p+2])<<8 | int(body[p+3])
		p += 4
		if p+el > end {
			break
		}
		switch et {
		case extECH:
			h.HasECH = true
		case extServerName:
			if sni, ok := parseSNIExt(body[p : p+el]); ok {
				h.SNI = sni
			}
		}
		p += el
	}
	return h, true
}

func parseSNIExt(b []byte) (string, bool) {

	if len(b) < 2 {
		return "", false
	}
	end := 2 + (int(b[0])<<8 | int(b[1]))
	if end > len(b) {
		end = len(b)
	}
	q := 2
	for q+3 <= end {
		nameType := b[q]
		nameLen := int(b[q+1])<<8 | int(b[q+2])
		q += 3
		if q+nameLen > end {
			break
		}
		if nameType == 0x00 {
			return string(b[q : q+nameLen]), true
		}
		q += nameLen
	}
	return "", false
}

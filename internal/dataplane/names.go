package dataplane

const (
	Table = "chaff"

	ChainForward = "forward"

	SetBadV4  = "bad_v4"
	SetBadMAC = "bad_mac"

	CtMarkAllow = 0x1
	CtMarkDeny  = 0x2
)

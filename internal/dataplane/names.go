package dataplane

import "fmt"

const (
	Table = "chaff"

	ChainForward = "forward"
	ChainGroups  = "groups"

	SetBadV4  = "bad_v4"
	SetBadMAC = "bad_mac"

	// GroupSetPrefix — префикс имён per-group наборов (grp<ID>_mac, grp<ID>_v4).
	GroupSetPrefix = "grp"

	CtMarkAllow = 0x1
	CtMarkDeny  = 0x2
)

func GroupMACSet(id int64) string { return fmt.Sprintf("%s%d_mac", GroupSetPrefix, id) }
func GroupV4Set(id int64) string  { return fmt.Sprintf("%s%d_v4", GroupSetPrefix, id) }

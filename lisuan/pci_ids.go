package lisuan

// This file is data, not logic.
//
// lisuanModels maps a Lisuan PCI device ID — the hex value
// /sys/class/drm/cardN/device/device reports, without the "0x" prefix — to the
// name the PCI ID database publishes for that part. It is the only reliable way
// to name a Lisuan card: the vendor database most distributions ship (hwdata on
// this review host among them) has no entry for vendor 0x4C54 yet, so lspci on a
// machine with this card prints the raw identifiers — "00:02.0 0300: 4c54:5000" —
// rather than a product name. The Intel collector lost a bug to exactly that
// string reaching a card title, so this table exists instead of a query.
//
// The values are the database's own strings, deliberately not editorialised into
// the "7G100" marketing name: the card has shipped in more than one memory
// configuration, and nothing in the PCI device ID says which one is fitted, so a
// name claiming a specific model would be a guess. When the database grows a new
// device ID, add its row here — the integrity test in lisuan_test.go fails if a
// row is malformed or loses the vendor prefix.
var lisuanModels = map[string]string{
	"5000": "LISUAN 7G100 Series Graphics",
	"5001": "LISUAN 7G100 Series Graphics",
	"5002": "LISUAN 7G100 Series Graphics",
	"5003": "LISUAN 7G100 Series Graphics",
	"5004": "LISUAN 7G100 Series Graphics",
	"5005": "LISUAN 7G100 Series Graphics",
	"5006": "LISUAN 7G100 Series Graphics",
}

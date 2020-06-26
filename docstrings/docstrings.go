package docstrings

//go:generate sh ../scripts/helpgen.sh

// KeyStrings - Struct for help string storage
type KeyStrings struct {
	Usage string
	Short string
	Long  string
}

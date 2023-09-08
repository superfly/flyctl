package haikunator

import (
	"math/rand"
	"strconv"
	"strings"

	"github.com/superfly/flyctl/helpers"
)

var adjectives = strings.Fields(`
	autumn hidden bitter misty silent empty dry dark summer
	icy delicate quiet white cool spring winter patient
	twilight dawn crimson wispy weathered blue billowing
	broken cold damp falling frosty green long late lingering
	bold little morning muddy old red rough still small
	sparkling thrumming shy wandering withered wild black
	young holy solitary fragrant aged snowy proud floral
	restless divine polished ancient purple lively nameless
`)
var nouns = strings.Fields(`
	waterfall river breeze moon rain wind sea morning
	snow lake sunset pine shadow leaf dawn glitter forest
	hill cloud meadow sun glade bird brook butterfly
	bush dew dust field fire flower firefly feather grass
	haze mountain night pond darkness snowflake silence
	sound sky shape surf thunder violet water wildflower
	wave water resonance sun log dream cherry tree fog
	frost voice paper frog smoke star
`)

type builder struct {
	tokRange  int
	delimiter string
}

type Builder interface {
	TokenRange(r int) Builder
	Delimiter(d string) Builder
	Build() string
	String() string
}

func choose(list []string) string {
	return list[rand.Intn(len(list))]
}

func Haikunator() Builder {
	return &builder{
		tokRange:  9999,
		delimiter: "-",
	}
}

func (b *builder) TokenRange(r int) Builder {
	newB := helpers.Clone(b)
	newB.tokRange = r
	return newB
}
func (b *builder) Delimiter(d string) Builder {
	newB := helpers.Clone(b)
	newB.delimiter = d
	return newB
}
func (b *builder) Build() string {
	sections := []string{
		choose(adjectives),
		choose(nouns),
	}
	if b.tokRange > 0 {
		sections = append(sections, strconv.Itoa(rand.Intn(b.tokRange)))
	}
	return strings.Join(sections, b.delimiter)
}
func (b *builder) String() string {
	return b.Build()
}

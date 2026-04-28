package haikunator

import (
	"crypto/rand"
	"math/big"
	rand2 "math/rand"
	"slices"
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
	amber gentle bright calm silver golden mellow radiant
	soft tranquil velvet lucid rosy tender dusky sunlit
	starlit moonlit windblown graceful mellowed vivid mellowing
	verdant russet glowing drifting rolling humming gleaming
	peaceful faithful agile noble tidy ambered airy cinder
	marbled lustrous dappled kind coral lilac copper willow
	brisk serene curious plucky jaunty earnest honeyed satin
	ivory azure ambergris evergreen rippling glimmering unfurling
	shimmering buoyant wistful
`)

var nouns = strings.Fields(`
	waterfall river breeze moon rain wind sea morning
	snow lake sunset pine shadow leaf dawn glitter forest
	hill cloud meadow sun glade bird brook butterfly
	bush dew dust field fire flower firefly feather grass
	haze mountain night pond darkness snowflake silence
	sound sky shape surf thunder violet water wildflower
	wave stone resonance branch log dream cherry tree fog
	frost voice paper frog smoke star
	ocean canyon pebble harbor valley blossom petal lantern
	comet aurora meadowlark shell driftwood cove ridge ember
	stream island harborlight seastar meadowland hillside raindrop starlight
	sunbeam moonbeam tide current lagoon harborbird skylark pinecone
	acorn grove orchard garden pathway meadowbrook songbird beacon
	marsh hollow coastline summit inlet woodland headland echo
	horizon overbrook snowfall moonrise sunrise tidepool sandbar fern
	willow reed coral shoreline song meadowstone harborwave glow
`)

type Builder struct {
	tokRange  int
	delimiter string

	RandN func(max int) int
}

func Haikunator() *Builder {
	return &Builder{
		tokRange:  9999,
		delimiter: "-",

		RandN: func(max int) int {
			ret, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
			if err != nil {
				// Fallback to "insecure" random
				// it doesn't really matter, this is not security critical
				return rand2.Intn(max) // skipcq: GSC-G404
			}

			return int(ret.Int64())
		},
	}
}

func (b *Builder) choose(list []string) string {
	return list[b.RandN(len(list))]
}

func (b *Builder) TokenRange(r int) *Builder {
	newB := helpers.Clone(b)
	newB.tokRange = r

	return newB
}

func (b *Builder) Delimiter(d string) *Builder {
	newB := helpers.Clone(b)
	newB.delimiter = d

	return newB
}

func (b *Builder) Build() string {
	sections := []string{
		b.choose(adjectives),
		b.choose(nouns),
	}
	if b.tokRange > 0 {
		sections = append(sections, strconv.Itoa(b.RandN(b.tokRange)))
	}

	return strings.Join(sections, b.delimiter)
}

func (b *Builder) String() string {
	return b.Build()
}

func GeneratedAppName() string {
	return GeneratedAppNameWithPrefix("")
}

func GeneratedAppNameWithPrefix(prefix string) string {
	builder := Haikunator().TokenRange(0)
	name := builder.Build()
	token := strconv.FormatInt(int64(builder.RandN(1000)+1000), 10)[1:]

	if prefix == "" {
		return strings.Join([]string{name, token}, "-")
	}

	return strings.Join([]string{prefix, name, token}, "-")
}

// TrimSuffix removes a haiku name at the end of s, if it exists.
// Otherwise returns the original string.
func (b *Builder) TrimSuffix(s string) string {
	a := strings.Split(s, b.delimiter)
	if len(a) < 3 {
		return s
	}

	adjective, noun, num := a[len(a)-3], a[len(a)-2], a[len(a)-1]
	if !slices.Contains(adjectives, adjective) {
		return s
	}
	if !slices.Contains(nouns, noun) {
		return s
	}
	if _, err := strconv.Atoi(num); err != nil {
		return s
	}

	return strings.Join(a[:len(a)-3], b.delimiter)
}

package vector

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

type SimilarityFunction struct {
	Identifier string
	Name       string
	UseCases   string
}

var similarityFunctions = []SimilarityFunction{
	{
		Identifier: "EUCLIDEAN",
		Name:       "Euclidean Distance",
		UseCases:   "Natural Language Processing, Recommendation Systems",
	},
	{
		Identifier: "COSINE",
		Name:       "Cosine Similarity",
		UseCases:   "Computer Vision, Anomaly Detection",
	},
	{
		Identifier: "DOT_PRODUCT",
		Name:       "Dot Product",
		UseCases:   "Machine Learning Models, Collaborative Filtering",
	},
}

type EmbeddingModel struct {
	Identifier string
	Name       string
	Dimensions int
}

var embeddingModels = []EmbeddingModel{
	{
		Identifier: "MXBAI_EMBED_LARGE_V1",
		Name:       "mixedbread-ai/mxbai-embed-large-v1",
		Dimensions: 1024,
	},
	{
		Identifier: "UAE_Large_V1",
		Name:       "WhereIsAI/UAE-Large-V1",
		Dimensions: 1024,
	},
	{
		Identifier: "BGE_LARGE_EN_V1_5",
		Name:       "BAAI/bge-large-en-v1.5",
		Dimensions: 1024,
	},
	{
		Identifier: "BGE_BASE_EN_V1_5",
		Name:       "BAAI/bge-base-en-v1.5",
		Dimensions: 768,
	},
	{
		Identifier: "BGE_SMALL_EN_V1_5",
		Name:       "BAAI/bge-small-en-v1.5",
		Dimensions: 384,
	},
	{
		Identifier: "ALL_MINILM_L6_V2",
		Name:       "sentence-transformers/all-MiniLM-L6-v2",
		Dimensions: 384,
	},
}

func New() (cmd *cobra.Command) {
	const (
		short = "Provision and manage Upstash Vector index"
		long  = short + "\n"
	)

	cmd = command.New("vector", short, long, nil)
	cmd.AddCommand(create(), list(), dashboard(), destroy(), status())

	return cmd
}

var SharedFlags = flag.Set{}

package vector

import "testing"

func TestEmbeddingModelForOptionMatchesPrompt(t *testing.T) {
	options := embeddingModelOptions()

	if got, want := len(options), len(embeddingModels)+1; got != want {
		t.Fatalf("got %d options, want %d", got, want)
	}

	if model := embeddingModelForOption(0); model != nil {
		t.Errorf("option 0 (%s) picked %s, want no model", options[0], model.Name)
	}

	for index := 1; index < len(options); index++ {
		model := embeddingModelForOption(index)
		if model == nil {
			t.Errorf("option %d (%s) picked no model", index, options[index])
			continue
		}
		if model.Name != options[index] {
			t.Errorf("option %d (%s) picked %s", index, options[index], model.Name)
		}
	}
}

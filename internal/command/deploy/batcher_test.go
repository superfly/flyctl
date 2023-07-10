package deploy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func prepBatch(total, groupCount int, soloFirst bool) []int {
	b := &batcher[int]{
		TotalJobs:  total,
		GroupCount: groupCount,
		SoloFirst:  soloFirst,
	}

	batches := []int{}
	for n := 0; n < total; n++ {
		b.Add(n)
		batch := b.Batch()
		if len(batch) > 0 {
			batches = append(batches, len(batch))
		}
	}

	return batches
}

func Test_batcher(t *testing.T) {
	assert.Equal(t, []int{1}, prepBatch(1, 1, true))
	assert.Equal(t, []int{1}, prepBatch(1, 1, false))
	assert.Equal(t, []int{1, 1}, prepBatch(2, 1, true))
	assert.Equal(t, []int{2}, prepBatch(2, 1, false))
	assert.Equal(t, []int{1, 1}, prepBatch(2, 2, true))
	assert.Equal(t, []int{1, 1}, prepBatch(2, 2, false))
	assert.Equal(t, []int{1, 1, 1}, prepBatch(3, 2, true))
	assert.Equal(t, []int{2, 1}, prepBatch(3, 2, false))
	assert.Equal(t, []int{1, 2, 2}, prepBatch(5, 2, true))
	assert.Equal(t, []int{3, 2}, prepBatch(5, 2, false))
	assert.Equal(t, []int{1, 2, 1, 1, 1, 1}, prepBatch(7, 5, true))
	assert.Equal(t, []int{2, 2, 1, 1, 1}, prepBatch(7, 5, false))
	assert.Equal(t, []int{1, 5, 5, 5, 4, 4}, prepBatch(24, 5, true))
	assert.Equal(t, []int{5, 5, 5, 5, 4}, prepBatch(24, 5, false))

	assert.Equal(t, []int{1, 2, 2, 2}, prepBatch(7, 3, true))
	assert.Equal(t, []int{1, 8, 8, 7}, prepBatch(24, 3, true))
}

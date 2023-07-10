package deploy

// batcher is a helper for adapting a series of jobs into batches of work.
// As you iterate through the jobs, you can call Add(jobInfo) to add a job to the current batch.
// At the end of each iteration, loop over Batch() to get the current batch of jobs.
// (it returns the current batch when it's ready to process)
// E.g.
//
//	for _, a := range items {
//	  itemToFurtherProcess := doSomething(a)
//	  batcher.Add(itemToFurtherProcess)
//	  for _, item := range batcher.Batch() {
//	     doSomethingElse(item)
//	  }
//	}
type batcher[T any] struct {
	// How many items in total we will process
	TotalJobs int
	// How many groups we want to split the jobs into
	GroupCount int
	// If true, the first job will be processed by itself
	SoloFirst bool

	// current index into TotalJobs
	idx int
	// current batch number (out of GroupCount)
	batchNum int
	// index into the current batch
	batchIdx int
	// the jobs in the current batch
	jobs []T
}

// Add a job to the current batch
func (b *batcher[T]) Add(job T) {
	b.jobs = append(b.jobs, job)
	b.idx++
	b.batchIdx++
}

// Ready returns true if the current batch is ready to be processed
func (b *batcher[T]) Ready() bool {
	// Normally index checks are 0-based, but at this point we've already added the job,
	// which increments the index. So, for the first go-around, idx will =1 here.
	if (b.SoloFirst && b.idx == 1) || b.idx == b.TotalJobs {
		return true
	}
	// If we solo'd the first job, ignore it in batch calculations
	batchedJobs := b.TotalJobs
	batchNum := b.batchNum
	if b.SoloFirst {
		batchedJobs--
		batchNum--
	}
	// Distribute the jobs evenly.
	// If there's a remainder N, the first N batches will have 1 extra job
	rem := batchedJobs % b.GroupCount
	groupSize := batchedJobs / b.GroupCount
	if batchNum < rem {
		groupSize++
	}
	return b.batchIdx == groupSize
}

// Batch returns the current batch if it's Ready, otherwise nil
// If the batch is returned, the batcher is reset
// It's safe to just always loop over the result of Batch - if it's not ready,
// the loop will exit immediately
func (b *batcher[T]) Batch() []T {
	if !b.Ready() {
		return nil
	}
	jobs := b.jobs
	b.jobs = nil
	b.batchNum++
	b.batchIdx = 0
	return jobs
}

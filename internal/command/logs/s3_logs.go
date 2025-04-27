package logs

import (
	"bufio"
	"container/heap"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/klauspost/compress/zstd"
	"golang.org/x/sync/errgroup"
)

// -----------------------------------------------------------------------------
// Domain models
// -----------------------------------------------------------------------------

type LogEntry struct {
	Timestamp time.Time
	Data      []byte
}

type objectMeta struct {
	key      string
	earliest time.Time // batchEnd – 5 min
}

// -----------------------------------------------------------------------------
// Stream & priority‑queue helpers
// -----------------------------------------------------------------------------

type Stream struct {
	entry LogEntry
	next  <-chan LogEntry
}

type streamHeap []Stream

func (h streamHeap) Len() int            { return len(h) }
func (h streamHeap) Less(i, j int) bool  { return h[i].entry.Timestamp.Before(h[j].entry.Timestamp) }
func (h streamHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *streamHeap) Push(x interface{}) { *h = append(*h, x.(Stream)) }
func (h *streamHeap) Pop() interface{}   { v := (*h)[h.Len()-1]; *h = (*h)[:h.Len()-1]; return v }

// thin wrapper to hide heap boilerplate

type StreamPQ struct{ h streamHeap }

func NewStreamPQ() *StreamPQ      { pq := &StreamPQ{}; heap.Init(&pq.h); return pq }
func (q *StreamPQ) Len() int      { return len(q.h) }
func (q *StreamPQ) Push(s Stream) { heap.Push(&q.h, s) }
func (q *StreamPQ) Pop() Stream   { return heap.Pop(&q.h).(Stream) }
func (q *StreamPQ) Peek() Stream  { return q.h[0] }

// -----------------------------------------------------------------------------
// S3 helpers
// -----------------------------------------------------------------------------

func listObjects(ctx context.Context, s3c *s3.Client, bucket string, from, to time.Time) ([]objectMeta, error) {
	var metas []objectMeta
	for _, p := range hourPrefixes(from.Add(-5*time.Minute), to) {
		pager := s3.NewListObjectsV2Paginator(s3c, &s3.ListObjectsV2Input{Bucket: &bucket, Prefix: &p})
		for pager.HasMorePages() {
			pg, err := pager.NextPage(ctx)
			if err != nil {
				return nil, err
			}
			for _, obj := range pg.Contents {
				key := *obj.Key
				parts := strings.SplitN(path.Base(key), "-", 2)
				epoch, err := strconv.ParseInt(parts[0], 10, 64)
				if err != nil {
					continue
				}
				end := time.Unix(epoch, 0)
				early := end.Add(-5 * time.Minute)
				if end.Before(from) || early.After(to) {
					continue
				}
				metas = append(metas, objectMeta{key, early})
			}
		}
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].earliest.Before(metas[j].earliest) })
	return metas, nil
}

func hourPrefixes(from, to time.Time) []string {
	from = from.Truncate(time.Hour)
	to = to.Truncate(time.Hour)
	var pfx []string
	for t := from; !t.After(to); t = t.Add(time.Hour) {
		pfx = append(pfx, fmt.Sprintf("date=%04d-%02d-%02d/hour=%02d/", t.Year(), t.Month(), t.Day(), t.Hour()))
	}
	return pfx
}

func openObject(ctx context.Context, s3c *s3.Client, bucket, key string, from, to time.Time) (Stream, error) {
	resp, err := s3c.GetObject(ctx, &s3.GetObjectInput{Bucket: &bucket, Key: &key})
	if err != nil {
		return Stream{}, err
	}
	dec, err := zstd.NewReader(resp.Body)
	if err != nil {
		return Stream{}, err
	}

	br := bufio.NewReaderSize(dec, 256<<10)
	jsonDec := json.NewDecoder(br)

	lineCh := make(chan LogEntry, 1)
	go func() {
		defer func() { dec.Close(); _ = resp.Body.Close(); close(lineCh) }()
		for {
			var raw json.RawMessage
			if err := jsonDec.Decode(&raw); err != nil {
				break
			}
			var meta struct{ Timestamp string }
			if err := json.Unmarshal(raw, &meta); err != nil {
				continue
			}
			ts, err := time.Parse(time.RFC3339Nano, meta.Timestamp)
			if err != nil {
				log.Printf("time parse error: %v", err)
				continue
			}
			ts = ts.UTC()
			if ts.Before(from) {
				continue
			}
			if ts.After(to) {
				break
			}
			lineCh <- LogEntry{Timestamp: ts, Data: append([]byte(nil), raw...)}
		}
	}()

	first, ok := <-lineCh
	if !ok {
		return Stream{}, fmt.Errorf("%s: no lines in window", key)
	}
	return Stream{entry: first, next: lineCh}, nil
}

// -----------------------------------------------------------------------------
// mergeObjects – explicit state, no nil‑channel toggling
// -----------------------------------------------------------------------------

func mergeObjects(ctx context.Context, s3c *s3.Client, bucket string, metas []objectMeta, from, to time.Time, out io.Writer) error {
	const maxConcurrent = 32

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrent)

	in := make(chan Stream, maxConcurrent)
	for i := range metas {
		m := metas[i]
		g.Go(func() error {
			stream, err := openObject(gctx, s3c, bucket, m.key, from, to)
			if err != nil {
				log.Printf("open %s: %v", m.key, err)
			} else {
				select {
				case in <- stream:
				case <-gctx.Done():
				}
			}
			return nil
		})
	}
	go func() { _ = g.Wait(); close(in) }()

	// buffered writer avoids extra goroutine
	bw := bufio.NewWriter(out)

	pq := NewStreamPQ()
	inOpen := true

	for inOpen || pq.Len() > 0 {
		if pq.Len() == 0 { // must wait on input only
			st, ok := <-in
			if !ok {
				inOpen = false
				continue
			}
			pq.Push(st)
			continue
		}

		select {
		case st, ok := <-in:
			if ok {
				pq.Push(st)
			} else {
				inOpen = false
			}
		case <-ctx.Done():
			return ctx.Err()
		default:
			// output path
			top := pq.Pop()
			if _, err := bw.Write(top.entry.Data); err != nil {
				return err
			}
			if err := bw.WriteByte('\n'); err != nil {
				return err
			}
			if nxt, ok := <-top.next; ok {
				pq.Push(Stream{entry: nxt, next: top.next})
			}
		}
	}

	if err := bw.Flush(); err != nil {
		return err
	}
	return g.Wait()
}

// -----------------------------------------------------------------------------
// Public API
// -----------------------------------------------------------------------------

func ProcessLogs(ctx context.Context, s3c *s3.Client, bucket string, from, to time.Time, out io.Writer) error {
	metas, err := listObjects(ctx, s3c, bucket, from, to)
	if err != nil {
		return err
	}
	if len(metas) == 0 {
		return nil
	}
	return mergeObjects(ctx, s3c, bucket, metas, from, to, out)
}

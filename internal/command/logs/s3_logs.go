package logs

import (
	"bufio"
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
)

type Object struct {
	key   string
	entry LogEntry
	ch    <-chan LogEntry
}

type LogEntry struct {
	Timestamp time.Time
	Data      []byte
}

func listObjects(ctx context.Context, s3c *s3.Client, bucket string, from, to time.Time) ([]*Object, error) {
	var objs []*Object
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
				batchEnd := time.Unix(epoch, 0).UTC()
				batchStart := batchEnd.Add(-5 * time.Minute)
				if batchEnd.Before(from) || batchStart.After(to) {
					continue // object completely outside window
				}
				objs = append(objs, &Object{
					key:   key,
					entry: LogEntry{Timestamp: batchEnd.Add(-5 * time.Minute).UTC()},
				})
			}
		}
	}
	sort.Slice(objs, func(i, j int) bool {
		return objs[i].entry.Timestamp.Before(objs[j].entry.Timestamp)
	})
	return objs, nil
}

func hourPrefixes(from, to time.Time) []string {
	from = from.Truncate(time.Hour)
	to = to.Truncate(time.Hour)
	var p []string
	for t := from; !t.After(to); t = t.Add(time.Hour) {
		p = append(p, fmt.Sprintf("date=%04d-%02d-%02d/hour=%02d/", t.Year(), t.Month(), t.Day(), t.Hour()))
	}
	return p
}

func (o *Object) open(ctx context.Context, s3c *s3.Client, bucket string, from, to time.Time) error {
	if o.ch != nil {
		return nil
	}

	resp, err := s3c.GetObject(ctx, &s3.GetObjectInput{Bucket: &bucket, Key: &o.key})
	if err != nil {
		return err
	}
	dec, err := zstd.NewReader(resp.Body)
	if err != nil {
		return err
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

	o.ch = lineCh
	return nil
}

func mergeObjects(ctx context.Context, s3c *s3.Client, bucket string, objects []*Object, from, to time.Time, out io.Writer) error {
	pq := NewPriorityQueue(func(a, b *Object) bool {
		return a.entry.Timestamp.Before(b.entry.Timestamp)
	})
	for _, obj := range objects {
		pq.Push(obj)
	}
	bw := bufio.NewWriter(out)
	defer bw.Flush()

	for pq.Len() > 0 {
		obj := pq.Pop()

		if obj.entry.Data == nil {
			if err := obj.open(ctx, s3c, bucket, from, to); err != nil {
				log.Printf("open %s: %v", obj.key, err)
				continue
			}
		} else {
			if _, err := bw.Write(obj.entry.Data); err != nil {
				return err
			}
			if err := bw.WriteByte('\n'); err != nil {
				return err
			}
		}
		next, ok := <-obj.ch
		if ok {
			obj.entry = next
			pq.Push(obj)
		}
	}
	return nil
}

func ProcessLogs(ctx context.Context, s3c *s3.Client, bucket string, from, to time.Time, out io.Writer) error {
	objects, err := listObjects(ctx, s3c, bucket, from, to)
	if err != nil {
		return err
	}
	if len(objects) == 0 {
		return nil
	}
	return mergeObjects(ctx, s3c, bucket, objects, from, to, out)
}

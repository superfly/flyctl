package logs

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/klauspost/compress/zstd"
)

type s3LogEntry struct {
	LogEntry
	Timestamp time.Time
	AppName   string
}

type Object struct {
	bucket string
	key    string
	start  time.Time
	entry  *s3LogEntry
	ch     <-chan s3LogEntry
}

func (o *Object) Time() time.Time {
	if o.entry != nil {
		return o.entry.Timestamp
	}
	return o.start
}
func (o *Object) Less(other *Object) bool {
	return o.Time().Before(other.Time())
}

func (s *s3Stream) listObjects(ctx context.Context, bucket string, rootPrefix string) ([]*Object, error) {
	var objs []*Object
	s.log.Debugf("Listing objects: %s %s", bucket, rootPrefix)
	for _, p := range hourPrefixes(s.opts.Start.Add(-5*time.Minute), s.opts.End) {
		p = rootPrefix + p
		s.log.Debugf("Listing objects: %s %s", bucket, p)
		pager := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{Bucket: &bucket, Prefix: &p})
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
					s.log.Errorf("Failed to parse epoch %s: %v", key, err)
					continue
				}
				batchEnd := time.Unix(epoch, 0).UTC()
				batchStart := batchEnd.Add(-5 * time.Minute)
				if batchEnd.Before(s.opts.Start) || batchStart.After(s.opts.End) {
					continue
				}
				objs = append(objs, &Object{bucket: bucket, key: key, start: batchStart})
			}
		}
	}
	s.log.Debugf("Found %d matching log objects", len(objs))
	return objs, nil
}

func (s *s3Stream) filter(log s3LogEntry) bool {
	return (s.opts.VMID == "" || s.opts.VMID == log.Meta.Instance) &&
		(s.opts.RegionCode == "" || s.opts.RegionCode == log.Region) &&
		(s.opts.AppName == "" || s.opts.AppName == log.AppName)
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

func (s *s3Stream) open(ctx context.Context, o *Object) bool {
	if o.ch != nil {
		return false
	}
	lineCh := make(chan s3LogEntry, 16)

	go func() {
		defer close(lineCh)

		resp, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: &o.bucket, Key: &o.key})
		if err != nil {
			s.log.Errorf("Failed to get object %s: %v", o.key, err)
			return
		}
		defer func() { _ = resp.Body.Close() }()

		dec, err := zstd.NewReader(resp.Body)
		if err != nil {
			return
		}
		defer dec.Close()

		scanner := bufio.NewScanner(dec)
		for scanner.Scan() {
			var entry natsLog
			if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
				s.log.Errorf("Failed to unmarshal line: %s\n%v", scanner.Text(), err)
				continue
			}
			te := s3LogEntry{LogEntry: logToEntry(&entry), AppName: entry.Fly.App.Name}

			// S3 objects have Hive-compatible timestamps; parse and convert to RFC3339
			te.Timestamp, err = time.Parse("2006-01-02 15:04:05.999999", te.LogEntry.Timestamp)
			if err != nil {
				continue
			}
			te.Timestamp = te.Timestamp.UTC()
			te.LogEntry.Timestamp = te.Timestamp.Format(time.RFC3339)
			if !s.filter(te) {
				continue
			}
			if te.Timestamp.Before(s.opts.Start) {
				continue
			}
			if te.Timestamp.After(s.opts.End) {
				break
			}
			lineCh <- te
		}
	}()

	o.ch = lineCh
	return true
}

const targetConcurrency = 100

func (s *s3Stream) streamObjects(ctx context.Context, objects []*Object, out chan<- LogEntry) error {
	opened := 0
	for _, o := range objects[:targetConcurrency] {
		s.open(ctx, o)
		opened++
	}

	pq := NewMinHeap(objects)
	for pq.Len() > 0 {
		obj := pq.PopMin()

		if obj.entry == nil {
			if s.open(ctx, obj) {
				opened++
			}
		} else {
			out <- obj.entry.LogEntry
		}
		next, ok := <-obj.ch
		if ok {
			obj.entry = &next
			pq.Insert(obj)
		} else {
			opened--
			if opened <= targetConcurrency {
				for _, o := range objects {
					if s.open(ctx, o) {
						opened++
						break
					}
				}
			}
		}
	}
	return nil
}

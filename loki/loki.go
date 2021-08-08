//go:generate protoc -I . --go_out=. logproto.proto
package loki

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/common/model"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	contentType  = "application/x-protobuf"
	postPath     = "/api/prom/push"
	maxErrMsgLen = 1024
)

type entry struct {
	labels model.LabelSet
	*Entry
}

type Loki struct {
	entry
	LokiURL   string
	BatchWait time.Duration
	BatchSize int
	lineChan  chan *logrus.Entry
	execEnv   string
	data      map[model.LabelName]model.LabelValue
	wg        sync.WaitGroup
}

func Init() {
	hook, err := New(viper.GetString("log.loki_url"), 102400, 1)
	if err != nil {
		logrus.Error(err)
	} else {
		logrus.AddHook(hook)
	}
}

func New(URL string, batchSize, batchWait int) (*Loki, error) {
	l := &Loki{
		LokiURL:   URL,
		BatchSize: batchSize,
		BatchWait: time.Duration(batchWait) * time.Second,
		lineChan:  make(chan *logrus.Entry, batchSize),
		data:      make(map[model.LabelName]model.LabelValue),
	}

	if execEnv, ok := os.LookupEnv("EXECUTION_ENVIRONMENT"); ok {
		l.execEnv = execEnv
	} else {
		l.execEnv = "test"
	}

	u, err := url.Parse(l.LokiURL)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(u.Path, postPath) {
		u.Path = postPath
		q := u.Query()
		u.RawQuery = q.Encode()
		l.LokiURL = u.String()
	}
	l.wg.Add(1)
	go l.run()
	return l, nil
}

func (l *Loki) Close() {
	close(l.lineChan)
	l.wg.Wait()
}

func (l *Loki) AddData(key, value string) {
	l.data[model.LabelName(key)] = model.LabelValue(value)
}

func (l *Loki) Fire(entry *logrus.Entry) error {
	l.lineChan <- entry
	return nil
}

func (l *Loki) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (l *Loki) run() {
	var (
		curPktTime  time.Time
		lastPktTime time.Time
		maxWait     = time.NewTimer(l.BatchWait)
		batch       = map[model.Fingerprint]*Stream{}
		batchSize   = 0
	)
	defer l.wg.Done()

	defer func() {
		if err := l.sendBatch(batch); err != nil {
			fmt.Fprintf(os.Stderr, "%v ERROR: loki flush: %v\n", time.Now(), err)
		}
	}()

	for {
		select {
		case ll, ok := <-l.lineChan:
			if !ok {
				return
			}
			curPktTime = ll.Time
			// guard against entry out of order errors
			if lastPktTime.After(curPktTime) {
				curPktTime = time.Now()
			}
			lastPktTime = curPktTime

			tsNano := curPktTime.UnixNano()
			ts := &timestamppb.Timestamp{
				Seconds: tsNano / int64(time.Second),
				Nanos:   int32(tsNano % int64(time.Second)),
			}

			l.entry = entry{model.LabelSet{}, &Entry{Timestamp: ts}}
			l.entry.labels["level"] = model.LabelValue(ll.Level.String())
			l.entry.labels["env"] = model.LabelValue(l.execEnv)
			for key, value := range l.data {
				l.entry.labels[key] = value
			}
			l.entry.Entry.Line, _ = ll.String()

			if batchSize+len(l.entry.Line) > l.BatchSize {
				if err := l.sendBatch(batch); err != nil {
					fmt.Fprintf(os.Stderr, "%v ERROR: send size batch: %v\n", lastPktTime, err)
				}
				batchSize = 0
				batch = map[model.Fingerprint]*Stream{}
				maxWait.Reset(l.BatchWait)
			}

			batchSize += len(l.entry.Line)
			fp := l.entry.labels.FastFingerprint()
			stream, ok := batch[fp]
			if !ok {
				stream = &Stream{
					Labels: l.entry.labels.String(),
				}
				batch[fp] = stream
			}
			stream.Entries = append(stream.Entries, l.Entry)

		case <-maxWait.C:
			if len(batch) > 0 {
				if err := l.sendBatch(batch); err != nil {
					fmt.Fprintf(os.Stderr, "%v ERROR: send time batch: %v\n", lastPktTime, err)
				}
				batchSize = 0
				batch = map[model.Fingerprint]*Stream{}
			}
			maxWait.Reset(l.BatchWait)
		}
	}
}

func (l *Loki) sendBatch(batch map[model.Fingerprint]*Stream) error {
	buf, err := encodeBatch(batch)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = l.send(ctx, buf)
	if err != nil {
		return err
	}
	return nil
}

func encodeBatch(batch map[model.Fingerprint]*Stream) ([]byte, error) {
	req := PushRequest{
		Streams: make([]*Stream, 0, len(batch)),
	}
	for _, stream := range batch {
		req.Streams = append(req.Streams, stream)
	}
	buf, err := proto.Marshal(&req)
	if err != nil {
		return nil, err
	}
	buf = snappy.Encode(nil, buf)
	return buf, nil
}

func (l *Loki) send(ctx context.Context, buf []byte) (int, error) {
	req, err := http.NewRequest("POST", l.LokiURL, bytes.NewReader(buf))
	if err != nil {
		return -1, err
	}
	req = req.WithContext(ctx)
	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return -1, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		scanner := bufio.NewScanner(io.LimitReader(resp.Body, maxErrMsgLen))
		line := ""
		if scanner.Scan() {
			line = scanner.Text()
		}
		err = fmt.Errorf("server returned HTTP status %s (%d): %s", resp.Status, resp.StatusCode, line)
	}
	return resp.StatusCode, err
}

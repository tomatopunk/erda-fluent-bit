package outerda

import (
	"bytes"
	"compress/gzip"
	"io"
	"math"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestBatchSender_SendLogEvent(t *testing.T) {
	logrus.SetLevel(logrus.InfoLevel)

	type fields struct {
		dataNum                     int
		batchEventSize              int
		timeoutTrigger              time.Duration
		waitDuration                time.Duration
		BatchEventLimit             int
		BatchEventContentLimitBytes int
	}
	type args struct {
		lg *LogEvent
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   int
	}{
		{
			name: "event limit trigger mode",
			fields: fields{
				dataNum:                     1000,
				batchEventSize:              10,
				timeoutTrigger:              time.Second,
				waitDuration:                time.Second * 2,
				BatchEventLimit:             10,
				BatchEventContentLimitBytes: math.MaxInt64,
			},
			args: args{
				lg: mockLogEvent,
			},
			want: 100,
		},
		{
			name: "content limit trigger mode",
			fields: fields{
				dataNum:                     1000,
				batchEventSize:              10,
				timeoutTrigger:              time.Second,
				waitDuration:                time.Second * 2,
				BatchEventLimit:             1001,
				BatchEventContentLimitBytes: len(mockLogEvent.Content) * 10,
			},
			args: args{
				lg: mockLogEvent,
			},
			want: 100,
		},
		{
			name: "timeout trigger mode",
			fields: fields{
				dataNum:                     1,
				batchEventSize:              10,
				timeoutTrigger:              time.Second,
				waitDuration:                time.Second * 2,
				BatchEventLimit:             10,
				BatchEventContentLimitBytes: math.MaxInt64,
			},
			args: args{
				lg: mockLogEvent,
			},
			want: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mm := mockRemote{}
			bs := NewBatchSender(batchConfig{
				send2remoteServer:           mm.SendContainerLog,
				BatchEventLimit:             tt.fields.BatchEventLimit,
				BatchTriggerTimeout:         tt.fields.timeoutTrigger,
				BatchEventContentLimitBytes: tt.fields.BatchEventContentLimitBytes,
			})
			for i := 0; i < tt.fields.dataNum; i++ {
				err := bs.SendLogEvent(tt.args.lg)
				if err != nil {
					t.Errorf("get error: %s", err)
				}
			}
			time.Sleep(tt.fields.waitDuration)

			assert.Equal(t, tt.want, mm.sendCount)
			assert.Equal(t, 0, bs.currentIndex)
			assert.Equal(t, 0, bs.currentContentSize)
		})
	}
}

func TestTimeoutTriggerTwice(t *testing.T) {
	// timeout trigger twice
	mm := mockRemote{}
	bs := NewBatchSender(batchConfig{
		send2remoteServer:           mm.SendContainerLog,
		BatchEventLimit:             10,
		BatchTriggerTimeout:         time.Second,
		BatchEventContentLimitBytes: len(mockLogEvent.Content) * 10,
	})
	t.Log("send first event")
	_ = bs.SendLogEvent(mockLogEvent)
	time.Sleep(time.Second * 3)
	assert.Equal(t, 0, bs.currentIndex)

	t.Log("send second event")
	_ = bs.SendLogEvent(mockLogEvent)
	time.Sleep(time.Second * 3)
	assert.Equal(t, 2, mm.sendCount)
	assert.Equal(t, 0, bs.currentIndex)
}

var mockLogEvent = &LogEvent{
	Source:    "container",
	ID:        "b2a9cb046a8275c57307cad907ef0a5553a78d6f4c1da7186566555d1a5383dd",
	Stream:    "stderr",
	Content:   "time=\"2021-10-12 16:00:14.130242184\" level=info msg=\"finish to run the task: executor K8S/MARATHONFORTERMINUSDEV (id: 1120384ca1, action: 5)\"\n",
	Timestamp: 1634025614130323755,
	Tags: map[string]string{
		"pod_name":              "scheduler-3feb156fc4-cf6b45b89-cwh5s",
		"pod_namespace":         "project-387-dev",
		"pod_id":                "ad05d65a-b8b0-4b7c-84f3-88a2abc11bde",
		"pod_ip":                "10.0.46.1",
		"container_id":          "b2a9cb046a8275c57307cad907ef0a5553a78d6f4c1da7186566555d1a5383dd",
		"dice_cluster_name":     "terminus-dev",
		"dice_application_name": "scheduler",
		"msp_env_id":            "abc111",
		"cluster_name":          "terminus-dev",
		"application_name":      "scheduler",
	},
}

type mockRemote struct {
	sendCount int
}

func (m *mockRemote) SendContainerLog(data []byte) error {
	m.sendCount++
	return nil
}

func (m *mockRemote) SendJobLog(data []byte) error {
	return nil
}

func TestBatchSender_flush(t *testing.T) {
	expected := make([]*LogEvent, 0)
	cfg := batchConfig{
		GzipLevel: 3,
		send2remoteServer: func(data []byte) error {
			expected = unmarshal(data)
			return nil
		},
	}
	bs := &BatchSender{
		batchLogEvent: make([]*LogEvent, cfg.BatchEventLimit),
		cfg:           cfg,
	}
	if cfg.GzipLevel > 0 {
		buf := bytes.NewBuffer(nil)
		gc, _ := gzip.NewWriterLevel(buf, cfg.GzipLevel)
		bs.compressor = &gzipper{
			buf:    buf,
			writer: gc,
		}
	}

	ass := assert.New(t)
	source := []*LogEvent{
		mockLogEvent,
	}
	err := bs.flush(source)
	ass.Nil(err)
	ass.Equal(expected, source)
}

func unmarshal(data []byte) []*LogEvent {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	out, err := io.ReadAll(gr)
	if err != nil {
		panic(err)
	}
	var res []*LogEvent
	err = json.Unmarshal(out, &res)
	if err != nil {
		panic(err)
	}
	return res
}

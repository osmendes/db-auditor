package report

import (
	"runtime/metrics"
	"sync"
	"time"
)

const heapObjectsMetric = "/memory/classes/heap/objects:bytes"

// HeapSampler records the peak Go heap (live + not yet swept objects) while a
// report is rendered (PF-07), so every job logs how much memory it really
// needed and the worker can stop a job that crosses its budget.
type HeapSampler struct {
	stop     chan struct{}
	done     chan struct{}
	mu       sync.Mutex
	baseline uint64
	peak     uint64
	onSample func(peakAboveBaseline uint64)
}

// StartHeapSampler samples the heap every interval until Stop. onSample, when
// set, receives the current peak above the starting heap after each sample.
func StartHeapSampler(interval time.Duration, onSample func(peakAboveBaseline uint64)) *HeapSampler {
	s := &HeapSampler{stop: make(chan struct{}), done: make(chan struct{}), onSample: onSample}
	s.baseline = readHeapObjects()
	s.peak = s.baseline
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-s.stop:
				return
			case <-ticker.C:
				s.sample()
			}
		}
	}()
	return s
}

func (s *HeapSampler) sample() {
	current := readHeapObjects()
	s.mu.Lock()
	if current > s.peak {
		s.peak = current
	}
	peak := s.peak - s.baseline
	s.mu.Unlock()
	if s.onSample != nil {
		s.onSample(peak)
	}
}

// Stop ends sampling and returns the peak heap growth above the baseline in bytes.
func (s *HeapSampler) Stop() uint64 {
	close(s.stop)
	<-s.done
	s.sample()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.peak - s.baseline
}

func readHeapObjects() uint64 {
	sample := []metrics.Sample{{Name: heapObjectsMetric}}
	metrics.Read(sample)
	if sample[0].Value.Kind() != metrics.KindUint64 {
		return 0
	}
	return sample[0].Value.Uint64()
}

// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package aggregator

import (
	"testing"
	"time"

	"github.com/m3db/m3cluster/kv/mem"
	"github.com/m3db/m3x/watch"

	"github.com/stretchr/testify/require"
	"github.com/uber-go/tally"
)

func TestFollowerFlushManagerOpen(t *testing.T) {
	doneCh := make(chan struct{})
	watchable := watch.NewWatchable()
	_, w, err := watchable.Watch()
	require.NoError(t, err)
	flushTimesManager := &mockFlushTimesManager{
		watchFlushTimesFn: func() (watch.Watch, error) {
			return w, nil
		},
	}
	opts := NewFlushManagerOptions().SetFlushTimesManager(flushTimesManager)
	mgr := newFollowerFlushManager(doneCh, opts).(*followerFlushManager)
	mgr.Open()

	watchable.Update(testFlushTimes)
	for {
		mgr.RLock()
		state := mgr.flushTimesState
		mgr.RUnlock()
		if state != flushTimesUninitialized {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.Equal(t, testFlushTimes, mgr.proto)
	close(doneCh)
	mgr.Close()
}

func TestFollowerFlushManagerCanNotLeadNotCampaigning(t *testing.T) {
	doneCh := make(chan struct{})
	electionManager := &mockElectionManager{
		isCampaigningFn: func() bool { return false },
	}
	opts := NewFlushManagerOptions().SetElectionManager(electionManager)
	mgr := newFollowerFlushManager(doneCh, opts).(*followerFlushManager)
	require.False(t, mgr.CanLead())
}

func TestFollowerFlushManagerCanNotLeadProtoNotUpdated(t *testing.T) {
	doneCh := make(chan struct{})
	electionManager := &mockElectionManager{
		isCampaigningFn: func() bool { return true },
	}
	opts := NewFlushManagerOptions().SetElectionManager(electionManager)
	mgr := newFollowerFlushManager(doneCh, opts).(*followerFlushManager)
	require.False(t, mgr.CanLead())
}

func TestFollowerFlushManagerCanNotLeadFlushWindowsNotEnded(t *testing.T) {
	doneCh := make(chan struct{})
	electionManager := &mockElectionManager{
		isCampaigningFn: func() bool { return true },
	}
	opts := NewFlushManagerOptions().SetElectionManager(electionManager)
	mgr := newFollowerFlushManager(doneCh, opts).(*followerFlushManager)
	mgr.proto = testFlushTimes
	mgr.openedAt = time.Unix(3624, 0)
	require.False(t, mgr.CanLead())
}

func TestFollowerFlushManagerCanLead(t *testing.T) {
	doneCh := make(chan struct{})
	electionManager := &mockElectionManager{
		isCampaigningFn: func() bool { return true },
	}
	opts := NewFlushManagerOptions().SetElectionManager(electionManager)
	mgr := newFollowerFlushManager(doneCh, opts).(*followerFlushManager)
	mgr.flushTimesState = flushTimesProcessed
	mgr.proto = testFlushTimes
	mgr.openedAt = time.Unix(3599, 0)
	require.True(t, mgr.CanLead())
}

func TestFollowerFlushManagerPrepareNoFlush(t *testing.T) {
	now := time.Unix(1234, 0)
	nowFn := func() time.Time { return now }
	doneCh := make(chan struct{})
	opts := NewFlushManagerOptions().
		SetMaxBufferSize(time.Minute).
		SetCheckEvery(time.Second)
	mgr := newFollowerFlushManager(doneCh, opts).(*followerFlushManager)
	mgr.nowFn = nowFn
	mgr.flushTimesState = flushTimesProcessed
	mgr.lastFlushed = now

	now = now.Add(time.Second)
	flushTask, dur := mgr.Prepare(testFlushBuckets)

	require.Nil(t, flushTask)
	require.Equal(t, time.Second, dur)
	require.Equal(t, now.Add(-time.Second), mgr.lastFlushed)
}

func TestFollowerFlushManagerPrepareFlushTimesUpdated(t *testing.T) {
	now := time.Unix(1234, 0)
	nowFn := func() time.Time { return now }
	doneCh := make(chan struct{})
	opts := NewFlushManagerOptions().
		SetMaxBufferSize(time.Minute).
		SetCheckEvery(time.Second)
	mgr := newFollowerFlushManager(doneCh, opts).(*followerFlushManager)
	mgr.nowFn = nowFn
	mgr.flushTimesState = flushTimesUpdated
	mgr.proto = testFlushTimes

	flushTask, dur := mgr.Prepare(testFlushBuckets)

	expected := []flushersGroup{
		{
			interval: time.Second,
			flushers: []flusherWithTime{
				{
					flusher:          testFlushBuckets[0].flushers[0],
					flushBeforeNanos: 3663000000000,
				},
				{
					flusher:          testFlushBuckets[0].flushers[1],
					flushBeforeNanos: 3668000000000,
				},
			},
		},
		{
			interval: time.Minute,
			flushers: []flusherWithTime{
				{
					flusher:          testFlushBuckets[1].flushers[0],
					flushBeforeNanos: 3660000000000,
				},
			},
		},
		{
			interval: time.Hour,
			flushers: []flusherWithTime{
				{
					flusher:          testFlushBuckets[2].flushers[0],
					flushBeforeNanos: 3600000000000,
				},
			},
		},
	}
	require.NotNil(t, flushTask)
	require.Equal(t, time.Duration(0), dur)
	task := flushTask.(*followerFlushTask)
	actual := task.flushersByInterval
	require.Equal(t, expected, actual)
	require.Equal(t, now, mgr.lastFlushed)
}

func TestFollowerFlushManagerPrepareMaxBufferSizeExceeded(t *testing.T) {
	now := time.Unix(1234, 0)
	nowFn := func() time.Time { return now }
	doneCh := make(chan struct{})
	opts := NewFlushManagerOptions().
		SetMaxBufferSize(time.Minute).
		SetForcedFlushWindowSize(10 * time.Second).
		SetCheckEvery(time.Second)
	mgr := newFollowerFlushManager(doneCh, opts).(*followerFlushManager)
	mgr.nowFn = nowFn
	mgr.flushTimesState = flushTimesProcessed
	mgr.lastFlushed = now

	// Advance time by forced flush window size and expect no flush because it's
	// not in forced flush mode.
	now = now.Add(10 * time.Second)
	flushTask, dur := mgr.Prepare(testFlushBuckets)
	require.Nil(t, flushTask)
	require.Equal(t, time.Second, dur)

	// Advance time by max buffer size and expect a flush.
	now = now.Add(time.Minute)
	flushTask, dur = mgr.Prepare(testFlushBuckets)

	expected := []flushersGroup{
		{
			interval: time.Second,
			flushers: []flusherWithTime{
				{
					flusher:          testFlushBuckets[0].flushers[0],
					flushBeforeNanos: 1244000000000,
				},
				{
					flusher:          testFlushBuckets[0].flushers[1],
					flushBeforeNanos: 1244000000000,
				},
			},
		},
		{
			interval: time.Minute,
			flushers: []flusherWithTime{
				{
					flusher:          testFlushBuckets[1].flushers[0],
					flushBeforeNanos: 1244000000000,
				},
			},
		},
		{
			interval: time.Hour,
			flushers: []flusherWithTime{
				{
					flusher:          testFlushBuckets[2].flushers[0],
					flushBeforeNanos: 1244000000000,
				},
			},
		},
	}
	require.NotNil(t, flushTask)
	require.Equal(t, time.Duration(0), dur)
	task := flushTask.(*followerFlushTask)
	actual := task.flushersByInterval
	require.Equal(t, expected, actual)
	require.Equal(t, now, mgr.lastFlushed)

	// Advance time by less than the forced flush window size and expect no flush.
	now = now.Add(time.Second)
	flushTask, dur = mgr.Prepare(testFlushBuckets)
	require.Nil(t, flushTask)
	require.Equal(t, mgr.checkEvery, dur)

	// Reset flush mode and advance time by forced flush window size and expect no
	// flush because it's no longer in forced flush mode.
	mgr.flushMode = kvUpdateFollowerFlush
	now = now.Add(10 * time.Second)
	flushTask, dur = mgr.Prepare(testFlushBuckets)
	require.Nil(t, flushTask)
	require.Equal(t, time.Second, dur)
}

func TestFollowerFlushManagerWatchFlushTimes(t *testing.T) {
	// Set up a flush times manager watching in-memory kv store.
	store := mem.NewStore()
	flushTimesManagerOpts := NewFlushTimesManagerOptions().
		SetFlushTimesKeyFmt(testFlushTimesKeyFmt).
		SetFlushTimesStore(store)
	flushTimesManager := NewFlushTimesManager(flushTimesManagerOpts)
	require.NoError(t, flushTimesManager.Open(testShardSetID))

	doneCh := make(chan struct{})
	opts := NewFlushManagerOptions().SetFlushTimesManager(flushTimesManager)
	mgr := newFollowerFlushManager(doneCh, opts).(*followerFlushManager)
	mgr.Open()

	// Update flush times and wait for change to propagate.
	_, err := store.Set(testFlushTimesKey, testFlushTimes)
	require.NoError(t, err)
	for {
		mgr.RLock()
		proto := mgr.proto
		flushTimesState := mgr.flushTimesState
		mgr.RUnlock()
		if flushTimesState == flushTimesUpdated {
			require.Equal(t, proto, testFlushTimes)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestFollowerFlushTaskRun(t *testing.T) {
	flushedBefore := make([]int64, 3)
	flushersByInterval := []flushersGroup{
		{
			duration: tally.NoopScope.Timer("foo"),
			flushers: []flusherWithTime{
				{
					flusher: &mockFlusher{
						discardBeforeFn: func(beforeNanos int64) { flushedBefore[0] = beforeNanos },
					},
					flushBeforeNanos: 1234,
				},
				{
					flusher: &mockFlusher{
						discardBeforeFn: func(beforeNanos int64) { flushedBefore[1] = beforeNanos },
					},
					flushBeforeNanos: 2345,
				},
			},
		},
		{
			duration: tally.NoopScope.Timer("bar"),
			flushers: []flusherWithTime{
				{
					flusher: &mockFlusher{
						discardBeforeFn: func(beforeNanos int64) { flushedBefore[2] = beforeNanos },
					},
					flushBeforeNanos: 3456,
				},
			},
		},
	}

	doneCh := make(chan struct{})
	opts := NewFlushManagerOptions()
	mgr := newFollowerFlushManager(doneCh, opts).(*followerFlushManager)
	flushTask := &followerFlushTask{
		mgr:                mgr,
		flushersByInterval: flushersByInterval,
	}
	flushTask.Run()
	require.Equal(t, []int64{1234, 2345, 3456}, flushedBefore)
}
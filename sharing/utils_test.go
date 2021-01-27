// Copyright (C) 2021 Storj Labs, Inc.
// See LICENSE for copying information.

package sharing

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"storj.io/common/testcontext"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func randSleep() {
	time.Sleep(time.Duration(rand.Int31n(50)) * time.Microsecond)
}

func TestMutexGroup(t *testing.T) {
	defer testcontext.NewWithTimeout(t, 5*time.Second).Cleanup()

	var muGroup MutexGroup
	var wg sync.WaitGroup
	var counters [3]*int32
	totalCounter := new(int32)
	for lockNo := 0; lockNo < len(counters); lockNo++ {
		counters[lockNo] = new(int32)
		for workerNo := 0; workerNo < 10; workerNo++ {
			wg.Add(1)
			go func(workerNo, lockNo int) {
				defer wg.Done()

				lockName := fmt.Sprint(lockNo)

				highwater := int32(0)

				for i := 0; i < 2000; i++ {
					randSleep()
					func() {
						unlock := muGroup.Lock(lockName)
						defer unlock()
						require.Equal(t, int32(1), atomic.AddInt32(counters[lockNo], 1))
						total := atomic.AddInt32(totalCounter, 1)
						require.LessOrEqual(t, total, int32(len(counters)))
						if total > highwater {
							highwater = total
						}
						randSleep()
						require.Equal(t, int32(0), atomic.AddInt32(counters[lockNo], -1))
						require.GreaterOrEqual(t, atomic.AddInt32(totalCounter, -1), int32(0))
					}()
				}

				require.Equal(t, highwater, int32(len(counters)))

			}(workerNo, lockNo)
		}
	}
	wg.Wait()

	require.Equal(t, int32(0), *totalCounter)
	for lockNo := 0; lockNo < len(counters); lockNo++ {
		require.Equal(t, int32(0), *counters[lockNo])
	}
}

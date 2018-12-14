// Copyright 2018 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package localstore

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/swarm/storage"
)

// TestDB_collectGarbage tests garbage collection runs
// by uploading and syncing a number of chunks.
func TestDB_collectGarbage(t *testing.T) {
	db, cleanupFunc := newTestDB(t, &Options{
		Capacity: 100,
	})
	defer cleanupFunc()

	testDB_collectGarbage(t, db)
}

// TestDB_collectGarbage_useRetrievalCompositeIndex tests
// garbage collection runs by uploading and syncing a number
// of chunks using composite retrieval index.
func TestDB_collectGarbage_useRetrievalCompositeIndex(t *testing.T) {
	db, cleanupFunc := newTestDB(t, &Options{
		Capacity:                   100,
		UseRetrievalCompositeIndex: true,
	})
	defer cleanupFunc()

	testDB_collectGarbage(t, db)
}

// TestDB_collectGarbage_multipleBatches tests garbage
// collection runs by uploading and syncing a number of
// chunks by having multiple smaller batches.
func TestDB_collectGarbage_multipleBatches(t *testing.T) {
	// lower the maximal number of chunks in a single
	// gc batch to ensure multiple batches.
	defer func(s int64) { gcBatchSize = s }(gcBatchSize)
	gcBatchSize = 2

	db, cleanupFunc := newTestDB(t, &Options{
		Capacity: 100,
	})
	defer cleanupFunc()

	testDB_collectGarbage(t, db)
}

// TestDB_collectGarbage_multipleBatches_useRetrievalCompositeIndex
// tests garbage collection runs by uploading and syncing a number
// of chunks using composite retrieval index and having multiple
// smaller batches.
func TestDB_collectGarbage_multipleBatches_useRetrievalCompositeIndex(t *testing.T) {
	// lower the maximal number of chunks in a single
	// gc batch to ensure multiple batches.
	defer func(s int64) { gcBatchSize = s }(gcBatchSize)
	gcBatchSize = 2

	db, cleanupFunc := newTestDB(t, &Options{
		Capacity:                   100,
		UseRetrievalCompositeIndex: true,
	})
	defer cleanupFunc()

	testDB_collectGarbage(t, db)
}

// testDB_collectGarbage is a helper test function to test
// garbage collection runs by uploading and syncing a number of chunks.
func testDB_collectGarbage(t *testing.T, db *DB) {
	uploader := db.NewPutter(ModePutUpload)
	syncer := db.NewSetter(ModeSetSync)

	chunkCount := 150

	testHookCollectGarbageChan := make(chan int64)
	defer setTestHookCollectGarbage(func(collectedCount int64) {
		testHookCollectGarbageChan <- collectedCount
	})()

	addrs := make([]storage.Address, 0)

	// upload random chunks
	for i := 0; i < chunkCount; i++ {
		chunk := generateRandomChunk()

		err := uploader.Put(chunk)
		if err != nil {
			t.Fatal(err)
		}

		err = syncer.Set(chunk.Address())
		if err != nil {
			t.Fatal(err)
		}

		addrs = append(addrs, chunk.Address())
	}

	gcTarget := db.gcTarget()

	var totalCollectedCount int64
	for {
		select {
		case c := <-testHookCollectGarbageChan:
			totalCollectedCount += c
		case <-time.After(10 * time.Second):
			t.Error("collect garbage timeout")
		}
		gcSize := atomic.LoadInt64(&db.gcSize)
		if gcSize == gcTarget {
			break
		}
	}

	wantTotalCollectedCount := int64(chunkCount) - gcTarget
	if totalCollectedCount != wantTotalCollectedCount {
		t.Errorf("total collected chunks %v, want %v", totalCollectedCount, wantTotalCollectedCount)
	}

	t.Run("pull index count", newIndexItemsCountTest(db.pullIndex, int(gcTarget)))

	t.Run("gc index count", newIndexItemsCountTest(db.gcIndex, int(gcTarget)))

	t.Run("gc size", newIndexGCSizeTest(db))

	// the first synced chunk should be removed
	t.Run("get the first synced chunk", func(t *testing.T) {
		_, err := db.NewGetter(ModeGetRequest).Get(addrs[0])
		if err != storage.ErrChunkNotFound {
			t.Errorf("got error %v, want %v", err, storage.ErrChunkNotFound)
		}
	})

	// last synced chunk should not be removed
	t.Run("get most recent synced chunk", func(t *testing.T) {
		_, err := db.NewGetter(ModeGetRequest).Get(addrs[len(addrs)-1])
		if err != nil {
			t.Fatal(err)
		}
	})
}

// TestDB_collectGarbage_withRequests tests garbage collection
// runs by uploading, syncing and requesting a number of chunks.
func TestDB_collectGarbage_withRequests(t *testing.T) {
	db, cleanupFunc := newTestDB(t, &Options{
		Capacity: 100,
	})
	defer cleanupFunc()

	testDB_collectGarbage_withRequests(t, db)
}

// TestDB_collectGarbage_withRequests_useRetrievalCompositeIndex
// tests garbage collection runs by uploading, syncing and
// requesting a number of chunks using composite retrieval index.
func TestDB_collectGarbage_withRequests_useRetrievalCompositeIndex(t *testing.T) {
	db, cleanupFunc := newTestDB(t, &Options{
		Capacity:                   100,
		UseRetrievalCompositeIndex: true,
	})
	defer cleanupFunc()

	testDB_collectGarbage_withRequests(t, db)
}

// testDB_collectGarbage_withRequests is a helper test function
// to test garbage collection runs by uploading, syncing and
// requesting a number of chunks.
func testDB_collectGarbage_withRequests(t *testing.T, db *DB) {
	uploader := db.NewPutter(ModePutUpload)
	syncer := db.NewSetter(ModeSetSync)

	testHookCollectGarbageChan := make(chan int64)
	defer setTestHookCollectGarbage(func(collectedCount int64) {
		testHookCollectGarbageChan <- collectedCount
	})()

	addrs := make([]storage.Address, 0)

	// upload random chunks just up to the capacity
	for i := 0; i < int(db.capacity)-1; i++ {
		chunk := generateRandomChunk()

		err := uploader.Put(chunk)
		if err != nil {
			t.Fatal(err)
		}

		err = syncer.Set(chunk.Address())
		if err != nil {
			t.Fatal(err)
		}

		addrs = append(addrs, chunk.Address())
	}

	// request the latest synced chunk
	// to prioritize it in the gc index
	// not to be collected
	_, err := db.NewGetter(ModeGetRequest).Get(addrs[0])
	if err != nil {
		t.Fatal(err)
	}

	// upload and sync another chunk to trigger
	// garbage collection
	chunk := generateRandomChunk()
	err = uploader.Put(chunk)
	if err != nil {
		t.Fatal(err)
	}
	err = syncer.Set(chunk.Address())
	if err != nil {
		t.Fatal(err)
	}
	addrs = append(addrs, chunk.Address())

	// wait for garbage collection

	gcTarget := db.gcTarget()

	var totalCollectedCount int64
	for {
		select {
		case c := <-testHookCollectGarbageChan:
			totalCollectedCount += c
		case <-time.After(10 * time.Second):
			t.Error("collect garbage timeout")
		}
		gcSize := atomic.LoadInt64(&db.gcSize)
		if gcSize == gcTarget {
			break
		}
	}

	wantTotalCollectedCount := int64(len(addrs)) - gcTarget
	if totalCollectedCount != wantTotalCollectedCount {
		t.Errorf("total collected chunks %v, want %v", totalCollectedCount, wantTotalCollectedCount)
	}

	t.Run("pull index count", newIndexItemsCountTest(db.pullIndex, int(gcTarget)))

	t.Run("gc index count", newIndexItemsCountTest(db.gcIndex, int(gcTarget)))

	t.Run("gc size", newIndexGCSizeTest(db))

	// requested chunk should not be removed
	t.Run("get requested chunk", func(t *testing.T) {
		_, err := db.NewGetter(ModeGetRequest).Get(addrs[0])
		if err != nil {
			t.Fatal(err)
		}
	})

	// the second synced chunk should be removed
	t.Run("get gc-ed chunk", func(t *testing.T) {
		_, err := db.NewGetter(ModeGetRequest).Get(addrs[1])
		if err != storage.ErrChunkNotFound {
			t.Errorf("got error %v, want %v", err, storage.ErrChunkNotFound)
		}
	})

	// last synced chunk should not be removed
	t.Run("get most recent synced chunk", func(t *testing.T) {
		_, err := db.NewGetter(ModeGetRequest).Get(addrs[len(addrs)-1])
		if err != nil {
			t.Fatal(err)
		}
	})
}

// setTestHookCollectGarbage sets testHookCollectGarbage and
// returns a function that will reset it to the
// value before the change.
func setTestHookCollectGarbage(h func(collectedCount int64)) (reset func()) {
	current := testHookCollectGarbage
	reset = func() { testHookCollectGarbage = current }
	testHookCollectGarbage = h
	return reset
}

// TestSetTestHookCollectGarbage tests if setTestHookCollectGarbage changes
// testHookCollectGarbage function correctly and if its reset function
// resets the original function.
func TestSetTestHookCollectGarbage(t *testing.T) {
	// Set the current function after the test finishes.
	defer func(h func(collectedCount int64)) { testHookCollectGarbage = h }(testHookCollectGarbage)

	// expected value for the unchanged function
	original := 1
	// expected value for the changed function
	changed := 2

	// this variable will be set with two different functions
	var got int

	// define the original (unchanged) functions
	testHookCollectGarbage = func(_ int64) {
		got = original
	}

	// set got variable
	testHookCollectGarbage(0)

	// test if got variable is set correctly
	if got != original {
		t.Errorf("got hook value %v, want %v", got, original)
	}

	// set the new function
	reset := setTestHookCollectGarbage(func(_ int64) {
		got = changed
	})

	// set got variable
	testHookCollectGarbage(0)

	// test if got variable is set correctly to changed value
	if got != changed {
		t.Errorf("got hook value %v, want %v", got, changed)
	}

	// set the function to the original one
	reset()

	// set got variable
	testHookCollectGarbage(0)

	// test if got variable is set correctly to original value
	if got != original {
		t.Errorf("got hook value %v, want %v", got, original)
	}
}
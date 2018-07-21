package main

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	WRITE_CODE        = "."
	WRITE_RETRY_CODE  = "|"
	SELECT_CODE       = "-"
	SELECT_RETRY_CODE = "|"
)

type RWLocker interface {
	sync.Locker
	RLock()
	RUnlock()
}

type FakeLocker struct{}

func (_ FakeLocker) Lock()    {}
func (_ FakeLocker) Unlock()  {}
func (_ FakeLocker) RLock()   {}
func (_ FakeLocker) RUnlock() {}

// MutexWrapper meets the RWLocker interface but just uses sync.Mutex for everything
type MutexWrapper struct {
	sync.Mutex
}

func (l *MutexWrapper) RLock()   { l.Lock() }
func (l *MutexWrapper) RUnlock() { l.Unlock() }

func main() {

	writerCount := 2
	readerCount := 2
	numRecords := 10
	numUpdates := 500

	// test it with no mutex, letting sqlite deal with it
	fmt.Println("Running no-mutex test")
	durNoMutex, err := runTest(writerCount, readerCount, numRecords, numUpdates, &FakeLocker{})
	if err != nil {
		log.Fatal(err.Error())
		return
	}

	// test it with golang's sync.Mutex
	// see: https://github.com/mattn/go-sqlite3/issues/274
	fmt.Println()
	fmt.Println()
	fmt.Println("Running sync.Mutex test")
	durMutex, err := runTest(writerCount, readerCount, numRecords, numUpdates, &MutexWrapper{})
	if err != nil {
		log.Fatal(err.Error())
		return
	}

	fmt.Println()
	fmt.Println()
	fmt.Println("Running sync.RWMutex test")
	durRWMutex, err := runTest(writerCount, readerCount, numRecords, numUpdates, &sync.RWMutex{})
	if err != nil {
		log.Fatal(err.Error())
		return
	}

	fmt.Println()
	fmt.Println()
	fmt.Println("Legend")
	fmt.Println("---------------------------")
	fmt.Println("Write       : ", WRITE_CODE)
	fmt.Println("Write Retry : ", WRITE_RETRY_CODE)
	fmt.Println("Read        : ", SELECT_CODE)
	fmt.Println("Read Retry  : ", SELECT_RETRY_CODE)
	fmt.Println()
	fmt.Println()
	fmt.Println("Timings:")
	fmt.Println("---------------------------")
	fmt.Printf("No Mutex     took: %v (base)\n", durNoMutex)
	fmt.Printf("sync.Mutex   took: %v diff (%v)\n", durMutex, durMutex-durNoMutex)
	fmt.Printf("sync.RWMutex took: %v diff (%v)\n", durRWMutex, durRWMutex-durNoMutex)

}

// runTests creates writerCount, readerCount goroutines to write/read to the
// database respectively.  It will create numRecords and then do numUpdates to them
// while constantly reading from the database as fast as possible.
// locker is the sync.Locker that will be used to lock the database at the go layer
func runTest(writerCount, readerCount, numRecords, numUpdates int, locker RWLocker) (time.Duration, error) {
	// open it in memory mode
	dsn := fmt.Sprintf("file:%d?mode=memory&cache=shared&_busy_timeout=100", time.Now().UnixNano())
	db, _ := sql.Open("sqlite3", dsn)
	defer db.Close()

	_, err := db.Exec(`
		CREATE TABLE testData(id integer primary key, value integer not null) WITHOUT ROWID;
		PRAGMA journal_mode = WAL;
	`)
	if err != nil {
		return 0, err
	}

	// fill the database with the records we will be using
	for i := 0; i <= numRecords; i++ {
		_, err := db.Exec("INSERT INTO testData(id, value) VALUES (?,0)", i)
		if err != nil {
			return 0, err
		}
	}

	var readerWG sync.WaitGroup
	stopReaders := make(chan bool)
	for r := 0; r < readerCount; r++ {
		readerWG.Add(1)
		go func(id int) {
			defer readerWG.Done()
			for {
				select {
				case <-stopReaders:
					return
				default:
					locker.RLock()
					for {
						rows, err := db.Query("SELECT * FROM testData")
						if err != nil {
							fmt.Print(SELECT_RETRY_CODE)
						} else {
							fmt.Print(SELECT_CODE)
							for rows.Next() {
								// just do some work ..
							}
							break
						}
					}
					locker.RUnlock()
				}
			}
		}(r)
	}

	var writerWG sync.WaitGroup
	// workChan is a queue that is consumed in parallel by writers
	// to update one of the rows in the database
	workChan := make(chan int, writerCount*2)
	for w := 0; w < writerCount; w++ {
		writerWG.Add(1)
		go func(id int) {
			defer writerWG.Done()
			for val := range workChan {
				if val == -1 { // abort all writers
					close(workChan)
					return
				}

				locker.Lock()

				for {
					_, err := db.Exec("UPDATE testData set value=? WHERE id=?", val, 1+rand.Intn(numRecords))
					if err != nil {
						fmt.Print(WRITE_RETRY_CODE)
						continue
					} else {
						fmt.Print(WRITE_CODE)
						break
					}
				}

				locker.Unlock()
			}
		}(w)
	}

	go func() {
		for i := 0; i < numUpdates; i++ {
			workChan <- rand.Intn(int(math.MaxUint32))
		}
		workChan <- -1 // stop signal
		fmt.Println()
	}()

	start := time.Now()
	writerWG.Wait()
	dur := time.Now().Sub(start)

	close(stopReaders)
	readerWG.Wait()

	return dur, nil
}

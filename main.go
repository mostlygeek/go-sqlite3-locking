package main

import (
	"database/sql"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
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

	testType := flag.String("type", "none", "Locking type: [none, mutex, rwmutex]")
	writerCount := flag.Int("writers", 2, "Number of parallel writers")
	readerCount := flag.Int("readers", 2, "Number of parallel readers ")
	numRows := flag.Int("rows", 10, "Number of total DB rows, lower number = more contention")
	numUpdates := flag.Int("updates", 500, "How many UPDATE dml operations to perform over numRows")
	flag.Parse()

	fmt.Println("Legend")
	fmt.Println("---------------------------")
	fmt.Println("Write       : ", WRITE_CODE)
	fmt.Println("Write Retry : ", WRITE_RETRY_CODE)
	fmt.Println("Read        : ", SELECT_CODE)
	fmt.Println("Read Retry  : ", SELECT_RETRY_CODE)
	fmt.Println()

	var dur time.Duration
	var err error
	switch *testType {
	case "none":
		fmt.Println("Running no-mutex test")
		dur, err = runTest(*writerCount, *readerCount, *numRows, *numUpdates, &FakeLocker{})
	case "mutex":
		fmt.Println("Running sync.Mutex test")
		dur, err = runTest(*writerCount, *readerCount, *numRows, *numUpdates, &MutexWrapper{})
	case "rwmutex":
		fmt.Println("Running sync.RWMutex test")
		dur, err = runTest(*writerCount, *readerCount, *numRows, *numUpdates, &sync.RWMutex{})
	default:
		fmt.Println("Invalid test type:", *testType)
	}

	if err != nil {
		fmt.Println("Error: ", err.Error())
		os.Exit(1)
	} else {
		fmt.Println()
		fmt.Println()
		fmt.Println("Duration: ", dur)
	}
}

// runTests creates writerCount, readerCount goroutines to write/read to the
// database respectively.  It will create numRows and then do numUpdates to them
// while constantly reading from the database as fast as possible.
// locker is the sync.Locker that will be used to lock the database at the go layer
func runTest(writerCount, readerCount, numRows, numUpdates int, locker RWLocker) (time.Duration, error) {
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
	for i := 0; i <= numRows; i++ {
		_, err := db.Exec("INSERT INTO testData(id, value) VALUES (?,0)", i)
		if err != nil {
			return 0, err
		}
	}

	var readerWG sync.WaitGroup
	stopReaders := make(chan bool)

	// read from the database as much/fast as possible
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
								// purge it
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
					_, err := db.Exec("UPDATE testData set value=? WHERE id=?", val, 1+rand.Intn(numRows))
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
	}()

	start := time.Now()
	writerWG.Wait()
	dur := time.Now().Sub(start)

	close(stopReaders)
	readerWG.Wait()

	return dur, nil
}

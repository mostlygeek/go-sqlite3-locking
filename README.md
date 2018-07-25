# Test sqlite3 locking situations

Testing performance and behaviour of doing locking in go's runtime vs sqlite's runtime.
The test sets up parallel readers and writers.  Each reader will try to read from the database as 
fast as possible.  Writers will clear a work queue of updates to do.  Print some interesting 
ASCII art of the access patterns. 


## Installing dependencies:

This will fetch and prebuild go-sqlit3:

`$ go install github.com/mattn/go-sqlite3`


## Running it:

```
# building it 
$ go build . -o test-sqlite

# cli help
$ ./test-sqlite -h
Usage of ./test-sqlite:
  -readers int
        Number of parallel readers  (default 2)
  -rows int
        Number of total DB rows, lower number = more contention (default 10)
  -type string
        Locking type: [none, mutex, rwmutex] (default "none")
  -updates int
        How many UPDATE dml operations to perform over numRows (default 500)
  -writers int
        Number of parallel writers (default 2)


# (default) no locking, retries 
$ ./test-sqlite 

# using sync.Mutex
$ ./test-sqlite -type mutex

# using sync.RWMutex
$ ./test-sqlite -type rwmutex
```

## Output

A lot of fun ASCII symbols will be printed for the test, one for each retry, write and read.  This makes it easier to visualize what's happening.

```
$ ./test-sqlite -type rwmutex -updates 25
Legend
---------------------------
Write       :  .
Write Retry :  |
Read        :  -
Read Retry  :  |

Running sync.RWMutex test
--.--.--.--.--.--.--.--.--.--.--.--.--.--.--.--.--.--.--.--.--.--.--.--.--.--

Duration:  4.106226ms
```

## Try it with: 

```
# more writers than readers
$ ./test-sqlite -updates 500 -writers 4 -readers 3 -type mutex
$ ./test-sqlite -updates 500 -writers 4 -readers 3 -type rwmutex

# more readers than writers
$ ./test-sqlite -updates 500 -writers 4 -readers 5 -type mutex
$ ./test-sqlite -updates 500 -writers 4 -readers 5 -type rwmutex
```
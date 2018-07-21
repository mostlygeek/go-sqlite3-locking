# Test sqlite3 locking situations

Testing performance and behaviour of doing locking in go's runtime vs sqlite's runtime. 

Installing dependencies: 

This will fetch and prebuild go-sqlit3: 

`$ go install github.com/mattn/go-sqlite3`


Running it: 

```
go run ./main.go
```

It will 

* run a test with no locking, depending on sqlite for concurrency control
* run a test with `sync.Mutex`
* run a test with `sync.RWMutex`

Output: 

A lot of fun ASCII symbols will be printed for each test, one for each retry, write and read.  This makes it easier to visualize what's happening. 

```
ASCII Legend
---------------------------
Write       :  .
Write Retry :  |
Read        :  -
Read Retry  :  |
```

It'll also output timing information showing which type of locking is fastest: 

```
Timings:
---------------------------
No Mutex     took: 1.08497534s (base)
sync.Mutex   took: 29.488841ms diff (-1.055486499s)
sync.RWMutex took: 579.551571ms diff (-505.423769ms)
```

Take a look at the code to tweak number of parallel writers, readers, records and update operations are done. 
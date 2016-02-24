package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"runtime"
	"strings"
	"sync"

	"github.com/gomemqueue/lfreequeue"
)

type memqueue struct {
	sync.RWMutex
	queue       *lfreequeue.Queue // lock-free queue
	readersCond *sync.Cond
	numReaders  int
	queueChan   []chan string
	closer      chan bool
	wakeup      chan bool
	queueCloser []chan bool
}

var queueList = make(map[string]*memqueue)
var numStreamers = 0

// condition variable

var port = flag.Int("port", 9191, "gomemqueue default port")

func main() {

	flag.Parse()
	runtime.GOMAXPROCS(runtime.NumCPU())
	http.HandleFunc("/memqueue/", HandleClientRequest)
	http.HandleFunc("/memqueuefile/", HandleFileRequest)
	http.HandleFunc("/memqueuestream/", HandleStreamRequest)
	log.Printf("Starting memqueue server on port %v", *port)
	s := &http.Server{
		Addr:           fmt.Sprintf(":%v", *port),
		MaxHeaderBytes: 1 << 20,
	}
	log.Fatal(s.ListenAndServe())
}

func (mq *memqueue) streamMessages() {

	log.Printf("Stream reader started ... ")
	var init bool
	var watchIterator *lfreequeue.WatchIterator
	var iter <-chan interface{}

L:
	for {
		mq.readersCond.L.Lock()
		if mq.numReaders == 0 {
			mq.readersCond.Wait()
			// only init the interators once
			if init == false {
				watchIterator = mq.queue.NewWatchIterator()
				iter = watchIterator.Iter()
				defer watchIterator.Close()
				init = true

			}
		}
		mq.readersCond.L.Unlock()

		// received a signal to wake up
		// read all messages from the queue and write them to the server

		select {
		case <-mq.wakeup:
			// unblock the select to prevent a dequeue
			// if a reader goes away while we were blocked on the
			// iterator
		case <-mq.closer:
			// notify
			for _, c := range mq.queueCloser {
				close(c)
			}
			break L
		case v := <-iter:
			mq.RLock()
			queueChan := mq.queueChan
			mq.RUnlock()
			for _, c := range queueChan {
				c <- v.(string)
			}

			log.Printf(" message %v", v)

		}
	}

	log.Printf("Stream reader closing ... ")
}

func HandleStreamRequest(w http.ResponseWriter, r *http.Request) {

	queueName := strings.TrimPrefix(r.URL.Path, "/memqueuestream/")
	if queueName == "" {
		fmt.Fprintln(w, "Queue name not provided")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		panic("expected http.ResponseWriter to be an http.Flusher")
	}

	notify := w.(http.CloseNotifier).CloseNotify()
	enc := json.NewEncoder(w)

	mq, found := queueList[queueName]
	if found == false {
		fmt.Fprintln(w, "Queue not found", queueName)
		return
	}

	// create a channel on which we will receive messages

	skipClose := false

	mc := make(chan string, 1024)
	closer := make(chan bool)
	mq.Lock()
	mq.queueChan = append(mq.queueChan, mc)
	mq.queueCloser = append(mq.queueCloser, closer)
	mq.Unlock()

	defer func() {
		mq.Lock()
		index := 0
		// get the current index for this queue channel
		for i, q := range mq.queueChan {
			if q == mc {
				index = i
			}
		}
		mq.queueChan = append(mq.queueChan[:index], mq.queueChan[index+1:]...)
		close(mc)
		mq.queueCloser = append(mq.queueCloser[:index], mq.queueCloser[index+1:]...)
		if skipClose == false {
			close(closer)
		}
		mq.Unlock()
	}()

	switch r.Method {
	case "GET":
		// signal a reader if it is currently blocked
		mq.numReaders++
		defer func() { mq.numReaders-- }()
		mq.readersCond.Signal()

		for {
			select {
			case <-notify:
				mq.wakeup <- true
				log.Printf(" Request closed. Exitting ")
				return
			case <-closer:
				// the stream reader closes this channel
				skipClose = true
				log.Printf(" Got close signal ")
				return
			case msg := <-mc:

				message := map[string]interface{}{"message": msg}
				err := enc.Encode(&message)
				if err != nil {
					log.Printf(" Error in write %v", err)
					// finish  up
					return
				}
				flusher.Flush()
			}
		}

	default:
		http.Error(w, "Not supported", http.StatusInternalServerError)
	}

}

func HandleFileRequest(w http.ResponseWriter, r *http.Request) {

	queueName := strings.TrimPrefix(r.URL.Path, "/memqueuefile/")
	if queueName == "" {
		fmt.Fprintln(w, "Queue name not provided")
		return
	}

	switch r.Method {
	case "POST":
		mq, found := queueList[queueName]
		if found == false {
			fmt.Fprintln(w, "Queue not found", queueName)
			return
		}
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			fmt.Fprintln(w, "Failed to read body", err)
			return
		}

		qe := strings.Split(string(body), "\n")
		for e := range qe {
			mq.queue.Enqueue(e)
		}
	default:
		http.Error(w, "Not supported", http.StatusInternalServerError)
	}

}

func HandleClientRequest(w http.ResponseWriter, r *http.Request) {

	queueName := strings.TrimPrefix(r.URL.Path, "/memqueue/")
	if queueName == "" {
		fmt.Fprintln(w, "Queue name not provided")
		return
	}

	switch r.Method {
	case "GET":
		mq, found := queueList[queueName]
		if found == false {
			http.Error(w, "Queue not found", http.StatusNotFound)
			return
		}

		handleGet(mq, w)
	case "POST":
		mq, found := queueList[queueName]
		if found == false {
			http.Error(w, "Queue not found", http.StatusNotFound)
			return
		}
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusInternalServerError)
			return
		}

		mq.queue.Enqueue(string(body))

	case "PUT":
		// create a new queue if one doesn't exist
		if _, found := queueList[queueName]; found == true {
			http.Error(w, "Queue already exists", http.StatusInternalServerError)
			return
		}

		mq := &memqueue{queue: lfreequeue.NewQueue(),
			readersCond: &sync.Cond{L: &sync.Mutex{}},
			queueChan:   make([]chan string, 0),
			queueCloser: make([]chan bool, 0),
			closer:      make(chan bool),
			wakeup:      make(chan bool)}

		queueList[queueName] = mq
		go mq.streamMessages()
		fmt.Fprintln(w, "New queue successfully created", queueName)

	case "DELETE":
		mq, found := queueList[queueName]
		if found == false {
			http.Error(w, "Queue not found", http.StatusNotFound)
			return
		}
		mq.readersCond.Signal()
		delete(queueList, queueName)
		close(mq.closer)

	default:
		// Give an error message.
		http.Error(w, "Not supported", http.StatusInternalServerError)
	}
}

func handleGet(mq *memqueue, w http.ResponseWriter) {
	// read all messages from the queue and write them to the server
	enc := json.NewEncoder(w)

	for msg := range mq.queue.Iter() {
		message := map[string]interface{}{"message": msg}
		enc.Encode(&message)
	}
}

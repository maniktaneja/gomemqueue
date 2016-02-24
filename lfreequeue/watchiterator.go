package lfreequeue

type WatchIterator struct {
	queue *Queue
	quit  chan int
}

func (w *WatchIterator) Iter() <-chan interface{} {
	c := make(chan interface{})
	go w.watchAndIterate(c)
	return c
}

func (w *WatchIterator) watchAndIterate(c chan<- interface{}) {
	for {
		datum, ok := w.queue.Dequeue()

		if !ok {
			notify := w.queue.Watch()

			select {
			case <-notify:
				continue
			case <-w.quit:
				go func() {
					<-notify
				}()
				goto endIteration
			}

		} else {
			c <- datum
		}
	}

endIteration:
	close(c)
}

func (w *WatchIterator) Close() {
	go func() {
		w.quit <- 1
	}()
}

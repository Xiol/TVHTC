package main

var tcQueue = make(chan *TVHJob, 32)
var shutdownManager = make(chan bool)

func Transcode(job *TVHJob) {
	tcQueue <- job
	return
}

func StartQueueManager(config *Config) {
	Log.Warning("Queue manager starting up.")
	go func() {
		for {
			select {
			case job := <-tcQueue:
				// do transcode
				tc := NewTranscodeJob(job, config)
				tc.Transcode()
				Log.Info("%v jobs remaining in queue.", QueueLength())
			case <-shutdownManager:
				Log.Warning("Queue manager shutting down.")
				return
			}
		}
	}()
}

// stop the manager goroutine after the current job has finished
// processing.
func StopQueueManager() {
	go func() {
		shutdownManager <- true
	}()
}

func QueueLength() int {
	return len(tcQueue)
}

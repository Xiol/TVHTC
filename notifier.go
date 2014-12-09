package main

type Notifier interface {
	Send(job *TranscodeJob) error
}

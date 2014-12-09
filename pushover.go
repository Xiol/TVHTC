package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const pushoverMessageAPI string = "https://api.pushover.net/1/messages.json"

type PushoverResponse struct {
	Status  int16  `json:"status"`
	Request string `json:"request"`
}

type PushoverNotifier struct {
	APIToken string
	User     string
	Priority int
}

func NewPushoverNotifier(token, user string, priority int) PushoverNotifier {
	return PushoverNotifier{
		APIToken: token,
		User:     user,
		Priority: priority,
	}
}

func (this PushoverNotifier) Send(job *TranscodeJob) error {
	title := job.Job.Title
	var msg string
	if len(job.Message) > 512 {
		msg = job.Message[0:508]
		msg = fmt.Sprintf("%v...", msg)
	} else {
		msg = job.Message
	}

	return this.push(msg, title)
}

func (this *PushoverNotifier) push(message, title string) error {
	if len(message) > 512 {
		Log.Warning("Pushover message was too long, not sending. (%v > 512)", len(message))
		return fmt.Errorf("Message too long.")
	}

	payload := url.Values{}

	payload.Add("token", this.APIToken)
	payload.Add("user", this.User)
	payload.Add("priority", strconv.Itoa(this.Priority))
	payload.Add("timestamp", strconv.Itoa(int(time.Now().Unix())))
	payload.Add("title", fmt.Sprintf("New Recording: %v", title))
	payload.Add("message", message)

	resp, err := http.PostForm(pushoverMessageAPI, payload)
	if err != nil {
		Log.Warning("Pushover notification failure: %v", err)
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		Log.Warning("Unable to read Pushover response body: %v", err)
		return err
	}

	if resp.StatusCode > 200 && resp.StatusCode < 500 {
		Log.Warning("Got status code %v from Pushover API: %v", resp.StatusCode, body)
		return fmt.Errorf("Got HTTP status %v from Pushover API: %v", resp.StatusCode, body)
	}

	if resp.StatusCode >= 500 {
		// TODO: >500 means the Pushover guys are having trouble, should try again after
		// at least 5 seconds have passed. At the minute I'll just return the error but
		// really this should retry after a pause. Don't want to just time.Sleep() here, though.
		Log.Warning("Got status code %v from Pushover API: %v", resp.StatusCode, body)
		return fmt.Errorf("Got HTTP status %v from Pushover API: %v", resp.StatusCode, body)
	}

	pr := PushoverResponse{}
	err = json.Unmarshal(body, &pr)
	if err != nil {
		Log.Warning("Could not unmarshal JSON document from Pushover API, error '%v': %v", err, body)
		// Have got a 200 OK at this point though, so is this a problem? Not sure what to do here.
	}

	if pr.Status != 1 {
		Log.Warning("Pushover API returned 200 OK but a status of %v. Notification may not have been sent.", pr.Status)
		return fmt.Errorf("Didn't get status=1 back from Pushover API (got %v). Notification may not have been sent.\n", pr.Status)
	}

	Log.Debug("Pushover notification sent, response: %v", string(body))
	return nil
}

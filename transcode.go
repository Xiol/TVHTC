package main

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type MediaType uint8

const (
	_                     = iota // leave 0 so can test if defined
	MEDIA_VIDEO MediaType = 1 + iota
	MEDIA_AUDIO
	MEDIA_UNKNOWN
)

type TranscodeJob struct {
	Job          *TVHJob
	TempFilename string
	TempPath     string
	Keep         bool
	Success      bool
	Rename       bool
	Type         MediaType
	Message      string
	Handlers     []Notifier
	Conf         *Config
	OldSize      int64
	NewSize      int64
	ElapsedTime  time.Duration
}

func NewTranscodeJob(job *TVHJob, conf *Config) TranscodeJob {
	return TranscodeJob{Job: job, Conf: conf}
}

// Figures out who needs a notification when this job completes.
// This doesn't feel right, so probably needs rewriting.
func (this *TranscodeJob) SetupNotifications() {
	var def *Person
	for _, v := range this.Conf.NotifyList {
		if v.IsDefault {
			def = v
		}
		if v.NotificationWanted(this.Job.Title) {
			if v.Pushover != "" {
				Log.Debug("Adding Pushover notification for '%v'", v.Pushover)
				po := NewPushoverNotifier(this.Conf.PushoverToken, v.Pushover, 0)
				this.Handlers = append(this.Handlers, po)
			}
			if v.Email != "" {
				// Add email handler
			}
		}
	}
	if len(this.Handlers) == 0 && def != nil {
		if def.Pushover != "" {
			Log.Debug("Adding default Pushover notification for '%v'", def.Pushover)
			this.Handlers = append(this.Handlers, NewPushoverNotifier(this.Conf.PushoverToken, def.Pushover, 0))
		}
	}
}

// Runs the Send() function on all the notifications configured for this job.
func (this *TranscodeJob) SendNotifications() error {
	Log.Info("Sending notifications for transcode job '%v'", this.Job.Title)
	errors := make([]string, 0)

	for i := range this.Handlers {
		err := this.Handlers[i].Send(this)
		if err != nil {
			errors = append(errors, err.Error())
		}
	}

	if len(errors) > 0 {
		err := fmt.Errorf("Got one or more errors from notification handlers: %v", strings.Join(errors, " | "))
		Log.Warning(err.Error())
		return err
	}
	return nil
}

// Examine the file extension to determine if we're dealing with an audio or
// video file. TVHeadend only seems to output .mkv or .ts files for video, and
// .mka for audio, so these are all we're going to handle.
func (this *TranscodeJob) DetermineType() error {
	if this.Type > 0 {
		// already figured out the type
		return nil
	}

	ext := filepath.Ext(this.Job.Filename)
	if ext == ".mkv" || ext == ".ts" {
		Log.Debug("Determined extension %v to be MEDIA_VIDEO", ext)
		this.Type = MEDIA_VIDEO
	} else if ext == ".mka" {
		Log.Debug("Determined extension %v to be MEDIA_AUDIO", ext)
		this.Type = MEDIA_AUDIO
	} else {
		this.Type = MEDIA_UNKNOWN
		return fmt.Errorf("Unknown media format, file extension: %v", ext)
	}

	return nil
}

// Remove the original file and rename the transcoded file from the
// temporary name into the original name. Mainly for videos as we're
// just re-encoding .mkv -> .mkv
func (this *TranscodeJob) DoRename() error {
	if this.Rename && !this.Keep {
		Log.Debug("Doing rename %v -> %v", this.TempPath, this.Job.Path)
		if err := os.Remove(this.Job.Path); err != nil {
			return err
		}
		if err := os.Rename(this.TempPath, this.Job.Path); err != nil {
			return err
		}
	}
	return nil
}

// Mainly for audio as we'll be going .mka -> .mp3, so we only want
// to remove the original file and leave new .mp3 file alone.
func (this *TranscodeJob) Cleanup() error {
	if !this.Rename && !this.Keep && this.Type == MEDIA_AUDIO {
		Log.Debug("Removing %v", this.Job.Path)
		if err := os.Remove(this.Job.Path); err != nil {
			return err
		}
	}
	return nil
}

// Fills in all the temporary filename stuff for transcoding
func (this *TranscodeJob) GenerateTranscodeName() error {
	if this.TempFilename != "" && this.TempPath != "" {
		return nil
	}

	if err := this.DetermineType(); err != nil {
		return err
	}

	if this.Type == MEDIA_VIDEO {
		this.Rename = true
		this.TempFilename = fmt.Sprintf("%v.mkv", this.randomString())
		Log.Debug("MEDIA_VIDEO temporary filename: %v", this.TempFilename)
	} else {
		ext := filepath.Ext(this.Job.Filename)
		// strip extension from filename
		basename := this.Job.Filename[0 : len(this.Job.Filename)-len(ext)]
		this.TempFilename = fmt.Sprintf("%v.mp3", basename)
		Log.Debug("MEDIA_AUDIO temporary filename: %v", this.TempFilename)
	}
	this.TempPath = fmt.Sprintf("%v/%v", filepath.Dir(this.Job.Path), this.TempFilename)
	Log.Debug("Generated temporary path: %v", this.TempPath)
	return nil
}

func (this *TranscodeJob) randomString() string {
	rand := []byte(strconv.Itoa(int(rand.Int31())))
	hash := sha256.New()
	hash.Write(rand)
	hash.Write([]byte(time.Now().String()))
	return fmt.Sprintf("%x", hash.Sum(nil))
}

// Do the actual transcoding! Should really be the only function you
// need to call directly once the struct has been populated with data.
func (this *TranscodeJob) Transcode() error {
	this.SetupNotifications()
	// If the recording job failed, just send a notification about it.
	if this.Job.Status != "OK" {
		Log.Warning("TVHeadend reporting that recording programme '%v' did not succeed: %v", this.Job.Title, this.Job.Status)
		this.SendNotifications()
		return nil
	}

	var err error
	var oldstats os.FileInfo
	if oldstats, err = os.Stat(this.Job.Path); err != nil {
		this.Message = "File no longer exists? Nothing done."
		Log.Warning("File '%v' no longer exists? Aborting transcode.", this.Job.Path)
		this.SendNotifications()
		return fmt.Errorf(this.Message)
	}
	this.OldSize = oldstats.Size()

	if err = this.GenerateTranscodeName(); err != nil {
		this.Message = fmt.Sprintf("Error: %v", err.Error())
		Log.Warning("Error: %v", err)
		this.SendNotifications()
		return fmt.Errorf(this.Message)
	}

	tcsettings := make([]string, 2)
	tcsettings[0] = "-i"
	tcsettings[1] = this.Job.Path
	switch this.Type {
	case MEDIA_AUDIO:
		tcsettings = append(tcsettings, strings.Split(this.Conf.TCSettings.Audio, " ")...)
	case MEDIA_VIDEO:
		tcsettings = append(tcsettings, strings.Split(this.Conf.TCSettings.Video, " ")...)
	default:
		// Shouldn't get here as it should be dealt with further up.
		return fmt.Errorf("Unknown format, unable to handle.")
	}
	tcsettings = append(tcsettings, "-y")
	tcsettings = append(tcsettings, this.TempPath)

	Log.Debug("Executing command ffmpeg with arguments: %+v", tcsettings)

	cmd := exec.Command("ffmpeg", tcsettings...)

	before := time.Now()
	out, err := cmd.CombinedOutput()
	this.ElapsedTime = time.Since(before)
	if err != nil {
		// TODO: Write this out to a temporary file instead, likely
		// to be too big to send via pushover
		this.Message = fmt.Sprintf("Error during transcode:\n\n%v", out)
		Log.Warning(this.Message)
		this.SendNotifications()
		return err
	}

	newstats, _ := os.Stat(this.TempPath)
	this.NewSize = newstats.Size()

	var path string
	if this.Type == MEDIA_AUDIO {
		path = this.Job.Path
	} else {
		path = this.Job.Path
	}
	Log.Debug("Path: %v", path)
	path = strings.Replace(path, this.Conf.TrimPath, "", -1)
	Log.Debug("Trim Path: %v", path)

	this.Message = fmt.Sprintf("Transcode completed in %.2f minutes (size change: %vB -> %vB). Path: %v",
		this.ElapsedTime.Minutes(), this.OldSize, this.NewSize, path)
	Log.Info(this.Message)
	this.Success = true
	err = this.DoRename()
	if err != nil {
		Log.Warning("Error during rename: %v", err)
	}
	err = this.Cleanup()
	if err != nil {
		Log.Warning("Error during cleanup: %v", err)
	}
	this.SendNotifications()

	return nil
}

package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/op/go-logging"
)

var Log = logging.MustGetLogger("tvhtc")
var logFormat = logging.MustStringFormatter("[%{time:15:04:05.000}] %{level:.4s} (%{shortfunc}) - %{message}")
var syslogFormat = logging.MustStringFormatter("%{level:.4s} (%{shortfunc}) - %{message}")

func main() {
	var configPath string
	flag.StringVar(&configPath, "c", "/etc/tvhtc.conf", "Path to configuration file")
	var debug bool
	flag.BoolVar(&debug, "d", false, "Enable debugging output to stdout")
	flag.Parse()

	if !debug {
		sb, err := logging.NewSyslogBackend("tvhtc")
		if err != nil {
			fmt.Printf("Fatal error setting up syslog logging: %v", err)
			os.Exit(1)
		}
		logging.SetBackend(sb)
		logging.SetLevel(logging.INFO, "tvhtc")
		logging.SetFormatter(syslogFormat)
	} else {
		logging.SetBackend(logging.NewLogBackend(os.Stderr, "", 0))
		logging.SetLevel(logging.DEBUG, "tvhtc")
		logging.SetFormatter(logFormat)
	}

	Log.Warning("TVHTC starting up. Using config file: %v", configPath)

	rand.Seed(time.Now().Unix())
	config := NewConfig()
	if err := config.Load(configPath); err != nil {
		fmt.Printf(err.Error())
		os.Exit(1)
	}

	db := NewDatabase()
	db.Open()
	defer db.Close()
	db.Initialise()
	Log.Info("Database connection successful.")
	err := db.Recover()
	if err != nil {
		Log.Fatal("Failed to recover jobs from database: %v", err)
	}

	if !debug {
		gin.SetMode(gin.ReleaseMode)
	}
	g := gin.Default()

	g.POST("/job", func(c *gin.Context) {
		job := &TVHJob{}
		c.Bind(job)
		Log.Info("Received new transcode job: %+v", job)
		var err error
		job.DBID, err = db.AddEntry(job)
		if err != nil {
			Log.Error(err.Error())
			c.JSON(500, gin.H{"status": "error", "message": err.Error()})
			return
		}
		Transcode(job)
		c.JSON(200, gin.H{"status": "ok"})
		return
	})

	g.GET("/incompletejobs", func(c *gin.Context) {
		jobs, err := db.IncompleteJobs()
		if err != nil {
			Log.Error(err.Error())
			c.JSON(500, gin.H{"message": err.Error()})
			return
		}
		c.JSON(200, gin.H{"jobs": jobs})
		return
	})

	g.GET("/memstats", func(c *gin.Context) {
		// memory stats
		ms := runtime.MemStats{}
		runtime.ReadMemStats(&ms)
		c.JSON(200, gin.H{"total_alloc": ms.TotalAlloc, "in_use": ms.Alloc})
		return
	})

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM, syscall.SIGUSR1)
	go func() {
		for {
			sig := <-signalChannel
			switch sig {
			case os.Interrupt, syscall.SIGTERM:
				Log.Warning("Caught signal, shutting down. %v jobs left in queue.", QueueLength())
				StopQueueManager()
				db.Close()
				os.Exit(0)
			case syscall.SIGUSR1:
				// Could probably make this neater, but dat technical debt tho
				ppn := make(map[string]int)
				for p, v := range config.NotifyList {
					ppn[p] = len(v.InterestedIn)
				}

				Log.Warning("Reloading configuration...")
				config.Load(configPath)

				npn := make(map[string]int)
				for p, v := range config.NotifyList {
					npn[p] = len(v.InterestedIn)
				}
				Log.Info("Configuration reloaded. Notifications changed:")
				for k, v := range npn {
					if _, ok := ppn[k]; !ok {
						Log.Info("New user: %s -> %d notifications", k, v)
					} else {
						Log.Info("User %s: %d -> %d", k, ppn[k], npn[k])
					}
				}
			}
		}
	}()

	StartQueueManager(&config, db)
	g.Run(":8998")
}
